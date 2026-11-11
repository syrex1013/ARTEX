---
name: cloud-enum
description: Cloud infrastructure enumeration skill. Invoke when assessing AWS, Azure, or GCP environments for misconfigurations and exposed resources during an authorized engagement.
---

# Cloud Infrastructure Enumeration

Systematic cloud asset discovery and misconfiguration assessment under an **authorized** engagement. Covers AWS, Azure, and GCP.

---

## Scope constraints

| Allowed | Prohibited |
|---|---|
| Read-only enumeration with provided credentials/keys | Writing, modifying, or deleting cloud resources |
| Identifying public-facing resources (S3, blobs, GCS) | Accessing data not needed to prove exposure |
| Misconfiguration checks (IAM, SGs, bucket policies) | Exfiltrating sensitive data beyond proof-of-concept |
| Credential validation | Privilege escalation without written approval |

---

## AWS

### Identity and permissions
```bash
aws sts get-caller-identity
aws iam get-account-authorization-details --output json > iam_dump.json
# Enumerate own permissions
enumerate-iam --access-key $AWS_ACCESS_KEY_ID --secret-key $AWS_SECRET_ACCESS_KEY
# Check for password policy
aws iam get-account-password-policy
```

### S3 exposure
```bash
# List buckets
aws s3 ls
# Check public bucket policy
for bucket in $(aws s3 ls | awk '{print $3}'); do
  aws s3api get-bucket-policy --bucket $bucket 2>/dev/null
  aws s3api get-bucket-acl --bucket $bucket 2>/dev/null
done
# Check for anonymous access
aws s3 ls s3://<BUCKET_NAME> --no-sign-request
```

### EC2 and networking
```bash
aws ec2 describe-instances --query 'Reservations[].Instances[].[InstanceId,PublicIpAddress,Tags[?Key==`Name`].Value|[0],State.Name]' --output table
aws ec2 describe-security-groups | python3 -c "import sys,json; [print(sg['GroupId'],r['IpProtocol'],r.get('FromPort','*'),r.get('ToPort','*'),ip['CidrIp']) for sg in json.load(sys.stdin)['SecurityGroups'] for r in sg.get('IpPermissions',[]) for ip in r.get('IpRanges',[]) if ip['CidrIp']=='0.0.0.0/0']"
# IMDSv1 enabled check (from EC2 instance)
curl -s http://169.254.169.254/latest/meta-data/iam/security-credentials/ --max-time 2
```

### IAM privilege escalation paths
Use `PMapper` to find privilege escalation paths:
```bash
pmapper graph create
pmapper analysis --include-services
pmapper query "preset privesc"
```

---

## Azure

### Initial enum
```bash
az account show
az account list
az group list --output table
# All resources
az resource list --output table
```

### Storage exposure
```bash
az storage account list --query '[].{name:name,publicAccess:allowBlobPublicAccess}' --output table
# Check for public containers
az storage container list --account-name <ACCOUNT> --auth-mode login 2>/dev/null
```

### Virtual machines and NSGs
```bash
az vm list --show-details --query '[].{name:name,publicIps:publicIps,state:powerState}' --output table
az network nsg list --query '[].{name:name,rg:resourceGroup}' --output table
# Check NSG rules allowing 0.0.0.0/0 inbound
az network nsg rule list --nsg-name <NSG> --resource-group <RG> --query '[?sourceAddressPrefix==`*` || sourceAddressPrefix==`0.0.0.0/0`]'
```

### App registrations and service principals
```bash
az ad app list --all --query '[].{displayName:displayName,appId:appId,replyUrls:replyUrls}'
az ad sp list --all --query '[].{displayName:displayName,appId:appId,enabled:accountEnabled}'
```

---

## GCP

### Project and IAM
```bash
gcloud projects list
gcloud iam roles list --project <PROJECT_ID>
gcloud projects get-iam-policy <PROJECT_ID> --format=json | python3 -c "import sys,json; data=json.load(sys.stdin); [print(b['role'], m) for b in data.get('bindings',[]) for m in b['members'] if 'allUsers' in b['members'] or 'allAuthenticatedUsers' in b['members']]"
```

### Storage buckets
```bash
gsutil ls
gsutil iam get gs://<BUCKET>
# Public buckets
gsutil ls gs://<BUCKET> --no-sign-request 2>/dev/null
```

### Compute
```bash
gcloud compute instances list
gcloud compute firewall-rules list --filter="allowed[].ports:*" --format="table(name,network,sourceRanges,allowed)"
```

---

## Cross-cloud: exposed services scanner
```bash
# Use cloudbrute or similar for subdomain-based bucket discovery
# Match naming patterns: <company>-prod, <company>-dev, <company>-backup
for suffix in prod dev staging backup logs data; do
  aws s3 ls s3://<company>-$suffix --no-sign-request 2>/dev/null && echo "FOUND: <company>-$suffix"
done
```

---

## Key tools
`aws cli`, `azure cli`, `gcloud`, `enumerate-iam`, `PMapper`, `ScoutSuite`, `Prowler`, `CloudSploit`, `truffleHog` (secrets in repos).

## Output format
```
## Cloud Enum Report — <PROVIDER> <PROJECT/ACCOUNT> — <DATE>
### Identity context (account id, permissions)
### Exposed public resources
### IAM misconfigurations (overly permissive roles, public access)
### Network exposure (SG/NSG rules allowing 0.0.0.0/0)
### Findings
  #### [CRITICAL/HIGH/MEDIUM/LOW] <Finding>
  - Resource: <ARN / resource id>
  - Evidence:
  - Remediation:
```
