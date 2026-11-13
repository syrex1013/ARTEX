# ARTEX on Azure Container Apps

`main.bicep` provisions a Poland Central Container Apps environment, private PostgreSQL Flexible Server, private database DNS/VNet, Azure Files data/key shares, Log Analytics, a managed identity and an ACR Basic registry. The app uses HTTPS ingress on 8787 and managed-identity image pulls. Database credentials are a secure deployment parameter and Container Apps secret, never an output. The recording proxy is internal to the container.

Deploy foundation with `deployApplication=false`, build/push `artex:<imageTag>` to the output registry, then redeploy with `deployApplication=true`. Keep databasePassword stable across deployments; generate it into a mode-0600 parameter file outside the repository. Never commit or upload that file. Existing local workspace, provider credentials and database contents are not automatically copied to Azure.

The application runs one replica. Persistent custom skills are seeded without overwriting existing files under `/app/data/skills`; signing keys live separately under `/app/keys`. `ARTEX_TRAFFIC_JOURNAL_MODE=DELETE` selects SQLite rollback journaling for the network-mounted capture index; the normal local default remains WAL. This setting does not make SQLite a distributed database. Do not run overlapping revisions/server instances against the same capture files. Stop/pause active work and deactivate the old revision before upgrades that could overlap access. PostgreSQL contains accounts, sessions, engagement records, reports, discussion and attachment bytes.

Defaults are a 2-vCPU/4-GiB app, General Purpose D2ds_v5 PostgreSQL with 32 GiB storage and seven-day backups, and LRS file storage. Azure charges apply while these resources exist; student subscription region/quota policy still applies. Configure an LLM through the authenticated administration UI after deployment. Initial account setup requires the one-time token in container logs.

Validation:

```sh
bicep build infra/azure/main.bicep --outfile /tmp/artex-template.json
az deployment group validate --resource-group artex-poland \
  --template-file /tmp/artex-template.json --parameters @/secure/path/artex.parameters.private.json
```

After deployment, check `/api/health`, fresh login, membership isolation, report/provider readiness and persistence across a controlled restart. Do not claim report inference works until a configured provider successfully completes a generation.

This deployment uses the existing Azure for Students subscription with its native `subscriptionPolicies.spendingLimit` verified as `On`. No pay-as-you-go upgrade or spending-limit removal is authorized. Student credits can be consumed; when credits are exhausted or expire, Azure disables services. This is a credit-funded service with a hard spending stop, not a promise of perpetual free uptime. Azure budgets/alerts alone are not a hard stop.

Poland Central rejected both attempted Burstable B1ms and B2s PostgreSQL SKUs despite listing them in capabilities. The template uses General Purpose D2ds_v5 for the next provisioning attempt; this consumes credit faster. Keep the hard spending limit enabled.
