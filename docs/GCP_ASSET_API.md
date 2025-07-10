# GCP Asset API Documentation

## Overview

Cloudlist supports two distinct approaches for GCP asset discovery:

1. **Organization-Level Asset API** - Uses Cloud Asset Inventory API for comprehensive organization-wide discovery
2. **Individual Service APIs** - Uses individual GCP service APIs for project-specific discovery

## Approach Comparison

| Feature | Organization-Level Asset API | Individual Service APIs |
|---------|----------------------------|------------------------|
| **Scope** | All projects in organization | Accessible projects only |
| **Speed** | Slower (~45s) | Faster (~23s) |
| **Coverage** | 220+ resources across org | 67 resources per project |
| **Required Permissions** | Cloud Asset Inventory roles | Individual service permissions |
| **Best For** | Comprehensive org audits | Fast project-specific scans |

## Organization-Level Asset API

### Required Permissions

The service account needs **organization-level** permissions:

```yaml
# Required IAM roles for Asset API approach
roles:
  - roles/cloudasset.viewer          # Core Asset API access
  - roles/resourcemanager.viewer     # List projects in organization
  
# Optional roles for enhanced data extraction:
  - roles/compute.viewer             # Better compute instance details
  - roles/dns.reader                 # DNS record details
  - roles/storage.objectViewer       # Storage bucket details
```

### Service Account Setup

#### 1. Create Organization-Level Service Account

```bash
# Create service account in a project
gcloud iam service-accounts create asset-viewer-sa \
    --display-name="CloudList Asset Viewer" \
    --description="Service account for organization-level asset discovery"

# Get the service account email
SA_EMAIL="asset-viewer-sa@YOUR-PROJECT-ID.iam.gserviceaccount.com"
ORG_ID="YOUR-ORGANIZATION-ID"
```

#### 2. Grant Organization-Level Permissions

```bash
# Core Asset API permission (REQUIRED)
gcloud organizations add-iam-policy-binding $ORG_ID \
    --member="serviceAccount:$SA_EMAIL" \
    --role="roles/cloudasset.viewer"

# Resource manager permission (REQUIRED) 
gcloud organizations add-iam-policy-binding $ORG_ID \
    --member="serviceAccount:$SA_EMAIL" \
    --role="roles/resourcemanager.viewer"

# Optional: Enhanced permissions for better data extraction
gcloud organizations add-iam-policy-binding $ORG_ID \
    --member="serviceAccount:$SA_EMAIL" \
    --role="roles/compute.viewer"
```

#### 3. Generate Service Account Key

```bash
gcloud iam service-accounts keys create asset-viewer-key.json \
    --iam-account=$SA_EMAIL
```

### Configuration Example

```yaml
- provider: gcp
  id: org-discovery
  organization_id: "123456789012"  # Your organization ID (REQUIRED)
  gcp_service_account_key: |
    {
      "type": "service_account",
      "project_id": "your-project-id",
      "private_key_id": "...",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "asset-viewer-sa@your-project-id.iam.gserviceaccount.com",
      "client_id": "...",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/asset-viewer-sa%40your-project-id.iam.gserviceaccount.com",
      "universe_domain": "googleapis.com"
    }
```

### Usage Examples

```bash
# Discover all assets across organization
./cloudlist -pc config.yaml -id org-discovery

# Discover specific services only
./cloudlist -pc config.yaml -id org-discovery -s compute,dns,s3

# Verbose output with discovery details
./cloudlist -pc config.yaml -id org-discovery -v

# Output only IP addresses
./cloudlist -pc config.yaml -id org-discovery | grep -E "^[0-9]+\."
```

## Supported Asset Types

### Organization-Level Asset API Discovery

The Asset API approach queries the following asset types that provide IP addresses or DNS names:

| Asset Type | Service | Description |
|------------|---------|-------------|
| `compute.googleapis.com/Instance` | compute | Compute Engine instances |
| `compute.googleapis.com/GlobalAddress` | compute | Global static IP addresses |
| `compute.googleapis.com/Address` | compute | Regional static IP addresses |
| `compute.googleapis.com/ForwardingRule` | compute | Load balancer forwarding rules |
| `dns.googleapis.com/ManagedZone` | dns | DNS managed zones |
| `dns.googleapis.com/ResourceRecordSet` | dns | DNS resource record sets |
| `storage.googleapis.com/Bucket` | s3 | Cloud Storage buckets |
| `run.googleapis.com/Service` | cloud-run | Cloud Run services |
| `cloudfunctions.googleapis.com/CloudFunction` | cloud-function | Cloud Functions |
| `container.googleapis.com/Cluster` | gke | GKE clusters |
| `tpu.googleapis.com/Node` | tpu | Cloud TPU nodes |
| `file.googleapis.com/Instance` | filestore | Filestore instances |

### Available Services

You can specify individual services or use 'all' for comprehensive discovery:

```bash
# Individual services
./cloudlist -pc config.yaml -id org-discovery -s compute,dns,gke

# New services added
./cloudlist -pc config.yaml -id org-discovery -s tpu,filestore

# All services (comprehensive discovery)
./cloudlist -pc config.yaml -id org-discovery -s all
```

### Service-to-Asset Mapping

| Service | Asset Types |
|---------|-------------|
| `compute` | Instance, GlobalAddress, Address, ForwardingRule |
| `dns` | ManagedZone, ResourceRecordSet |
| `s3` | Bucket |
| `cloud-run` | Service |
| `cloud-function` | CloudFunction |
| `gke` | Cluster |
| `tpu` | Node |
| `filestore` | Instance |
| `all` | All asset types above |

## Individual Service APIs

### Required Permissions

The service account needs **project-level** permissions for each service:

```yaml
# Required IAM roles for Individual API approach
roles:
  - roles/compute.viewer              # Compute instances
  - roles/dns.reader                  # DNS records
  - roles/storage.objectViewer        # Storage buckets
  - roles/run.viewer                  # Cloud Run services  
  - roles/cloudfunctions.viewer       # Cloud Functions
  - roles/container.viewer            # GKE clusters
  - roles/resourcemanager.viewer      # List projects
```

### Service Account Setup

```bash
# Create service account
gcloud iam service-accounts create cloudlist-sa \
    --display-name="CloudList Individual Services" \
    --description="Service account for individual service API discovery"

SA_EMAIL="cloudlist-sa@YOUR-PROJECT-ID.iam.gserviceaccount.com"
PROJECT_ID="YOUR-PROJECT-ID"

# Grant project-level permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:$SA_EMAIL" \
    --role="roles/compute.viewer"

gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:$SA_EMAIL" \
    --role="roles/dns.reader"

# ... repeat for other services
```

### Configuration Example

```yaml
- provider: gcp
  id: project-discovery
  # No organization_id = uses individual service APIs
  gcp_service_account_key: |
    {
      "type": "service_account",
      "project_id": "your-project-id",
      "private_key_id": "...",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "cloudlist-sa@your-project-id.iam.gserviceaccount.com",
      "client_id": "...",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/cloudlist-sa%40your-project-id.iam.gserviceaccount.com",
      "universe_domain": "googleapis.com"
    }
```

## Finding Your Organization ID

```bash
# Method 1: Using gcloud
gcloud organizations list

# Method 2: Using Cloud Console
# Navigate to: IAM & Admin > Settings
# Organization ID is displayed at the top

# Method 3: Using Asset API (if you have access)
gcloud asset search-all-resources \
    --scope=organizations/YOUR-ORG-ID \
    --asset-types=cloudresourcemanager.googleapis.com/Organization
```

## Troubleshooting

### Common Permission Issues

**Error: "The caller does not have permission"**
```bash
# Verify organization-level permissions
gcloud organizations get-iam-policy YOUR-ORG-ID \
    --filter="bindings.members:serviceAccount:your-sa@project.iam.gserviceaccount.com"

# Check if Cloud Asset API is enabled
gcloud services list --enabled --filter="name:cloudasset.googleapis.com"
```

**Error: "No RESOURCE found that matches asset type"**
```bash
# Some asset types may not exist in your organization
# This is normal and can be ignored - the tool continues with other types
```

### API Quotas and Limits

- **Asset API**: 100 requests per 100 seconds per user
- **Rate Limiting**: Tool automatically batches requests to avoid quota issues
- **Timeout**: Organization discovery may take 30-60 seconds for large organizations

### Debugging

```bash
# Enable verbose logging
./cloudlist -pc config.yaml -id org-discovery -v

# Test Asset API access directly
gcloud asset list --organization=YOUR-ORG-ID --limit=5

# Verify service account permissions
gcloud auth activate-service-account --key-file=service-account-key.json
gcloud asset list --organization=YOUR-ORG-ID --limit=1
```

## Security Considerations

### Principle of Least Privilege

- **Asset API**: Grant only `cloudasset.viewer` and `resourcemanager.viewer`
- **Individual APIs**: Grant only the specific service viewer roles needed
- **Scope**: Prefer project-level permissions when organization-wide access isn't required

### Key Management

```bash
# Rotate service account keys regularly
gcloud iam service-accounts keys list --iam-account=$SA_EMAIL

# Delete old keys
gcloud iam service-accounts keys delete KEY-ID --iam-account=$SA_EMAIL

# Generate new key
gcloud iam service-accounts keys create new-key.json --iam-account=$SA_EMAIL
```

## Multiple Organization Support

Cloudlist supports discovering assets from **multiple GCP organizations simultaneously** by configuring multiple provider blocks in the same configuration file.

### Configuration Example

```yaml
# Multiple Organizations Configuration
- provider: gcp
  id: org-production
  organization_id: "111111111111"  # Production organization
  gcp_service_account_key: |
    {
      "type": "service_account",
      "project_id": "prod-project",
      "private_key_id": "...",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "asset-viewer-sa@prod-project.iam.gserviceaccount.com",
      "client_id": "...",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/asset-viewer-sa%40prod-project.iam.gserviceaccount.com",
      "universe_domain": "googleapis.com"
    }

- provider: gcp
  id: org-staging
  organization_id: "222222222222"  # Staging organization
  gcp_service_account_key: |
    {
      "type": "service_account",
      "project_id": "staging-project",
      "private_key_id": "...",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "asset-viewer-sa@staging-project.iam.gserviceaccount.com",
      "client_id": "...",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/asset-viewer-sa%40staging-project.iam.gserviceaccount.com",
      "universe_domain": "googleapis.com"
    }

- provider: gcp
  id: org-development
  organization_id: "333333333333"  # Development organization
  gcp_service_account_key: |
    {
      "type": "service_account",
      "project_id": "dev-project",
      "private_key_id": "...",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "asset-viewer-sa@dev-project.iam.gserviceaccount.com",
      "client_id": "...",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/asset-viewer-sa%40dev-project.iam.gserviceaccount.com",
      "universe_domain": "googleapis.com"
    }
```

### Usage Examples

```bash
# Discover assets from ALL organizations
./cloudlist -pc config.yaml -s all

# Discover assets from specific organization
./cloudlist -pc config.yaml -id org-production -s compute

# Discover assets from multiple specific organizations
./cloudlist -pc config.yaml -id org-production,org-staging -s compute

# Compare production vs staging environments
./cloudlist -pc config.yaml -id org-production -s all > prod-assets.txt
./cloudlist -pc config.yaml -id org-staging -s all > staging-assets.txt
diff prod-assets.txt staging-assets.txt
```

### Service Account Requirements

Each organization requires its own service account with appropriate permissions:

- **Organization-Level Permissions**: `roles/cloudasset.viewer` + `roles/resourcemanager.viewer`
- **Cross-Organization Setup**: Service accounts can be in different projects/organizations
- **Permission Inheritance**: Each service account only accesses its configured organization