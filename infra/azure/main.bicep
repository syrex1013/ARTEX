targetScope = 'resourceGroup'

param location string = 'polandcentral'
param prefix string = 'artex'
param imageTag string = 'collaboration'
param deployApplication bool = false
@secure()
param databasePassword string

var suffix = uniqueString(resourceGroup().id)
var registryName = '${prefix}${suffix}'
var storageName = '${prefix}${suffix}'

resource registry 'Microsoft.ContainerRegistry/registries@2023-07-01' = {
 name: registryName
 location: location
 sku: { name: 'Basic' }
 properties: { adminUserEnabled: false }
}
resource identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
 name: '${prefix}-app'
 location: location
}
resource registryPull 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
 name: guid(registry.id, identity.id, 'AcrPull')
 scope: registry
 properties: {
  principalId: identity.properties.principalId
  principalType: 'ServicePrincipal'
  roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', '7f951dda-4ed3-4680-a7ca-43fe172d538d')
 }
}
resource network 'Microsoft.Network/virtualNetworks@2023-11-01' = {
 name: '${prefix}-network'
 location: location
 properties: {
  addressSpace: { addressPrefixes: ['10.46.0.0/16'] }
  subnets: [
   {
    name: 'apps'
    properties: {
     addressPrefix: '10.46.0.0/23'
     delegations: [{ name: 'apps', properties: { serviceName: 'Microsoft.App/environments' } }]
     serviceEndpoints: [{ service: 'Microsoft.Storage' }]
    }
   }
   {
    name: 'database'
    properties: {
     addressPrefix: '10.46.2.0/24'
     delegations: [{ name: 'postgres', properties: { serviceName: 'Microsoft.DBforPostgreSQL/flexibleServers' } }]
    }
   }
  ]
 }
}
resource databaseDNS 'Microsoft.Network/privateDnsZones@2020-06-01' = {
 name: '${prefix}.private.postgres.database.azure.com'
 location: 'global'
}
resource databaseDNSLink 'Microsoft.Network/privateDnsZones/virtualNetworkLinks@2020-06-01' = {
 parent: databaseDNS
 name: '${prefix}-network'
 location: 'global'
 properties: { registrationEnabled: false, virtualNetwork: { id: network.id } }
}
resource postgres 'Microsoft.DBforPostgreSQL/flexibleServers@2024-08-01' = {
 name: '${prefix}-${suffix}'
 location: location
 sku: { name: 'Standard_D2ds_v5', tier: 'GeneralPurpose' }
 properties: {
  administratorLogin: 'artexadmin'
  administratorLoginPassword: databasePassword
  version: '16'
  storage: { storageSizeGB: 32 }
  backup: { backupRetentionDays: 7, geoRedundantBackup: 'Disabled' }
  highAvailability: { mode: 'Disabled' }
  network: {
   delegatedSubnetResourceId: '${network.id}/subnets/database'
   privateDnsZoneArmResourceId: databaseDNS.id
   publicNetworkAccess: 'Disabled'
  }
 }
 dependsOn: [databaseDNSLink]
}
resource database 'Microsoft.DBforPostgreSQL/flexibleServers/databases@2024-08-01' = {
 parent: postgres
 name: 'artex'
 properties: { charset: 'UTF8', collation: 'en_US.utf8' }
}
resource storage 'Microsoft.Storage/storageAccounts@2023-05-01' = {
 name: storageName
 location: location
 kind: 'StorageV2'
 sku: { name: 'Standard_LRS' }
 properties: {
  minimumTlsVersion: 'TLS1_2'
  supportsHttpsTrafficOnly: true
  allowBlobPublicAccess: false
  networkAcls: {
   defaultAction: 'Deny'
   bypass: 'AzureServices'
   virtualNetworkRules: [{ id: '${network.id}/subnets/apps', action: 'Allow' }]
  }
 }
}
resource fileService 'Microsoft.Storage/storageAccounts/fileServices@2023-05-01' = {
 parent: storage
 name: 'default'
}
resource shares 'Microsoft.Storage/storageAccounts/fileServices/shares@2023-05-01' = [for name in ['data', 'keys']: {
 parent: fileService
 name: name
 properties: { shareQuota: 100, enabledProtocols: 'SMB' }
}]
resource logs 'Microsoft.OperationalInsights/workspaces@2023-09-01' = {
 name: '${prefix}-logs'
 location: location
 properties: { sku: { name: 'PerGB2018' }, retentionInDays: 30 }
}
resource environment 'Microsoft.App/managedEnvironments@2024-03-01' = {
 name: '${prefix}-environment'
 location: location
 properties: {
  workloadProfiles: [{ name: 'Consumption', workloadProfileType: 'Consumption' }]
  vnetConfiguration: { infrastructureSubnetId: '${network.id}/subnets/apps', internal: false }
  appLogsConfiguration: {
   destination: 'log-analytics'
   logAnalyticsConfiguration: { customerId: logs.properties.customerId, sharedKey: logs.listKeys().primarySharedKey }
  }
 }
}
resource mounts 'Microsoft.App/managedEnvironments/storages@2024-03-01' = [for (name, i) in ['data', 'keys']: {
 parent: environment
 name: name
 properties: {
  azureFile: {
   accountName: storage.name
   accountKey: storage.listKeys().keys[0].value
   shareName: shares[i].name
   accessMode: 'ReadWrite'
  }
 }
}]
resource app 'Microsoft.App/containerApps@2024-03-01' = if (deployApplication) {
 name: prefix
 location: location
 identity: { type: 'UserAssigned', userAssignedIdentities: { '${identity.id}': {} } }
 properties: {
  managedEnvironmentId: environment.id
  workloadProfileName: 'Consumption'
  configuration: {
   activeRevisionsMode: 'Single'
   ingress: { external: true, targetPort: 8787, transport: 'http', allowInsecure: false }
   registries: [{ server: registry.properties.loginServer, identity: identity.id }]
   secrets: [{ name: 'postgres-dsn', value: 'postgres://artexadmin:${databasePassword}@${postgres.properties.fullyQualifiedDomainName}:5432/artex?sslmode=require' }]
  }
  template: {
   scale: { minReplicas: 1, maxReplicas: 1 }
   containers: [{
    name: prefix
    image: '${registry.properties.loginServer}/artex:${imageTag}'
    resources: { cpu: 2, memory: '4Gi' }
    command: ['/bin/sh', '-c']
    args: ['mkdir -p /app/data/skills && cp -an /app/skills/. /app/data/skills/ && exec /app/start.sh -addr :8787 -proxy :8788 -data /app/data']
    env: [
     { name: 'ARTEX_PG_DSN', secretRef: 'postgres-dsn' }
     { name: 'ARTEX_JWT_KEY_DIR', value: '/app/keys' }
     { name: 'ARTEX_SKILL_DIR', value: '/app/data/skills' }
     { name: 'ARTEX_TRAFFIC_JOURNAL_MODE', value: 'DELETE' }
    ]
    volumeMounts: [{ volumeName: 'data', mountPath: '/app/data' }, { volumeName: 'keys', mountPath: '/app/keys' }]
    probes: [
     { type: 'Startup', httpGet: { path: '/api/health', port: 8787 }, initialDelaySeconds: 5, periodSeconds: 10, failureThreshold: 30 }
     { type: 'Readiness', httpGet: { path: '/api/health', port: 8787 }, periodSeconds: 10, failureThreshold: 3 }
     { type: 'Liveness', httpGet: { path: '/api/health', port: 8787 }, periodSeconds: 30, failureThreshold: 3 }
    ]
   }]
   volumes: [for name in ['data', 'keys']: { name: name, storageType: 'AzureFile', storageName: name }]
  }
 }
 dependsOn: [mounts, registryPull, database]
}
output registryName string = registry.name
output registryServer string = registry.properties.loginServer
output environmentName string = environment.name
output applicationUrl string = deployApplication ? 'https://${app!.properties.configuration.ingress.fqdn}' : ''
