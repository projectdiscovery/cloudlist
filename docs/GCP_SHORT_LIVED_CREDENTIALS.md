# GCP Short-lived Credentials

This guide explains how to use GCP short-lived credentials with cloudlist for enhanced security.

## Overview

Short-lived credentials provide temporary access tokens (1-12 hours) instead of relying on static, long-lived service account keys. This approach:

- **Reduces security risk**: Tokens automatically expire, minimizing blast radius if compromised
- **Eliminates credential distribution**: Developers can use `gcloud auth login` instead of sharing key files
- **Enables least-privilege CI/CD**: Use minimal-permission keys for impersonation vs. full-access static keys
- **Improves compliance**: Aligns with zero-trust and credential rotation best practices
- **Provides audit trail**: Each execution generates fresh tokens with proper GCP logging

## How It Works

1. **Source Credentials**: cloudlist authenticates using one of:
   - Application Default Credentials (ADC) from `gcloud auth login`
   - Explicit service account key file
   - Workload Identity (GKE/Compute Engine)

2. **Token Generation**: cloudlist calls the IAM Service Account Credentials API to generate a short-lived access token for the target service account

3. **Impersonation**: The short-lived token grants access as the target service account

4. **Auto-refresh**: If operations exceed token lifetime, cloudlist automatically generates a new token

## Configuration Examples

### 1. Developer Workflow (Zero Keys)

Perfect for local development - no key files to manage!

```yaml
- provider: gcp
  id: dev-discovery
  use_short_lived_credentials: true
  service_account_email: "cloudlist@project.iam.gserviceaccount.com"
```

**Setup:**
```bash
# One-time authentication
gcloud auth application-default login

# Grant your user account permission to impersonate the service account
gcloud iam service-accounts add-iam-policy-binding \
  cloudlist@project.iam.gserviceaccount.com \
  --member="user:your-email@company.com" \
  --role="roles/iam.serviceAccountTokenCreator"

# Run cloudlist
cloudlist -config config.yaml
```

### 2. CI/CD with Minimal Permissions

Use a minimal-permission key that can only impersonate, not access resources directly.

```yaml
- provider: gcp
  id: ci-discovery
  use_short_lived_credentials: true
  service_account_email: "powerful-sa@project.iam.gserviceaccount.com"
  source_credentials: "minimal-ci-sa.json"
  token_lifetime: "7200s"  # 2 hours
```

**Setup:**
```bash
# Create minimal CI service account (only for impersonation)
gcloud iam service-accounts create minimal-ci-sa \
  --display-name="Minimal CI Service Account"

# Grant impersonation permission
gcloud iam service-accounts add-iam-policy-binding \
  powerful-sa@project.iam.gserviceaccount.com \
  --member="serviceAccount:minimal-ci-sa@project.iam.gserviceaccount.com" \
  --role="roles/iam.serviceAccountTokenCreator"

# Create key for CI
gcloud iam service-accounts keys create minimal-ci-sa.json \
  --iam-account=minimal-ci-sa@project.iam.gserviceaccount.com
```

### 3. GKE/Compute Engine (Zero Secrets)

Use workload identity - no credentials in config at all!

```yaml
- provider: gcp
  id: workload-discovery
  use_short_lived_credentials: true
  service_account_email: "cloudlist@project.iam.gserviceaccount.com"
```

**Setup (GKE):**
```bash
# Enable workload identity on cluster
gcloud container clusters update CLUSTER_NAME \
  --workload-pool=PROJECT_ID.svc.id.goog

# Bind Kubernetes service account to GCP service account
gcloud iam service-accounts add-iam-policy-binding \
  cloudlist@PROJECT_ID.iam.gserviceaccount.com \
  --role=roles/iam.workloadIdentityUser \
  --member="serviceAccount:PROJECT_ID.svc.id.goog[NAMESPACE/KSA_NAME]"

# Configure workload identity on deployment
kubectl annotate serviceaccount KSA_NAME \
  iam.gke.io/gcp-service-account=cloudlist@PROJECT_ID.iam.gserviceaccount.com
```

### 4. Migration from Existing Keys

Generate short-lived tokens from existing service account keys.

```yaml
- provider: gcp
  id: migrating-discovery
  use_short_lived_credentials: true
  service_account_email: "cloudlist@project.iam.gserviceaccount.com"
  gcp_service_account_key: '{
    "type": "service_account",
    "project_id": "your-project-id",
    ...
  }'
  token_lifetime: "3600s"
```

This allows you to maintain existing configs while gaining short-lived token benefits.

## IAM Permissions Required

### Source Credentials (What Generates Tokens)

Grant one of:
- **Role**: `roles/iam.serviceAccountTokenCreator`
- **Permission**: `iam.serviceAccounts.generateAccessToken`

**Command:**
```bash
gcloud iam service-accounts add-iam-policy-binding \
  TARGET_SA@PROJECT.iam.gserviceaccount.com \
  --member="user:YOUR_EMAIL@company.com" \
  --role="roles/iam.serviceAccountTokenCreator"
```

### Target Service Account (What Gets Impersonated)

Grant the same viewer roles required for traditional authentication:
- `roles/compute.viewer`
- `roles/dns.reader`
- `roles/storage.objectViewer`
- `roles/run.viewer`
- `roles/cloudfunctions.viewer`
- `roles/container.viewer`
- `roles/resourcemanager.viewer`

### Enable Required APIs

```bash
# Enable IAM Service Account Credentials API
gcloud services enable iamcredentials.googleapis.com
```

## Configuration Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `use_short_lived_credentials` | boolean | No | `false` | Enable short-lived token generation |
| `service_account_email` | string | Yes (if short-lived) | - | Target service account to impersonate |
| `source_credentials` | string | No | ADC | Path to source credentials file |
| `token_lifetime` | string | No | `"3600s"` | Token lifetime (1s to 43200s). Formats: "3600s", "1h", "1h30m" |
| `gcp_service_account_key` | string | No | - | Service account key JSON (works with short-lived mode) |

## Token Lifetime

**Supported formats:**
- Seconds: `"3600s"` (1 hour)
- Go duration: `"1h"`, `"30m"`, `"1h30m"`

**Limits:**
- Minimum: `1s`
- Maximum: `43200s` (12 hours)
- Default: `3600s` (1 hour)

**Considerations:**
- Shorter tokens = more secure but more API calls
- Longer tokens = fewer API calls but higher risk if leaked
- Recommended: 1-2 hours for most use cases

## Troubleshooting

### Error: "permission denied: ensure source credentials have 'roles/iam.serviceAccountTokenCreator'"

**Cause:** Source credentials don't have permission to impersonate target service account.

**Fix:**
```bash
gcloud iam service-accounts add-iam-policy-binding \
  TARGET_SA@PROJECT.iam.gserviceaccount.com \
  --member="TYPE:IDENTIFIER" \
  --role="roles/iam.serviceAccountTokenCreator"
```

Replace `TYPE:IDENTIFIER` with:
- `user:email@company.com` (for user accounts)
- `serviceAccount:sa@project.iam.gserviceaccount.com` (for service accounts)

### Error: "service account not found: verify the service_account_email is correct"

**Cause:** Target service account doesn't exist or is misspelled.

**Fix:**
```bash
# List service accounts to verify
gcloud iam service-accounts list

# Verify the email format: NAME@PROJECT.iam.gserviceaccount.com
```

### Error: "IAM Service Account Credentials API may not be enabled or accessible"

**Cause:** IAM Credentials API is not enabled.

**Fix:**
```bash
gcloud services enable iamcredentials.googleapis.com
```

### Error: "failed to find default google credentials"

**Cause:** No source credentials available and ADC not configured.

**Fix (for developers):**
```bash
gcloud auth application-default login
```

**Fix (for CI/CD):**
- Provide `source_credentials` parameter with path to service account key
- Or provide `gcp_service_account_key` in config

### Error: "token lifetime must be between 1 second and 12 hours"

**Cause:** `token_lifetime` is outside valid range.

**Fix:**
- Use a value between `"1s"` and `"43200s"`
- Common values: `"3600s"` (1h), `"7200s"` (2h), `"14400s"` (4h)

## Security Best Practices

1. **Use shortest practical token lifetime**
   - Development: 1 hour
   - CI/CD: 2-4 hours
   - Long-running operations: up to 12 hours

2. **Implement least privilege**
   - Source credentials: Only `roles/iam.serviceAccountTokenCreator`
   - Target service account: Only required viewer roles

3. **Use workload identity when possible**
   - No credentials in config files
   - Automatically managed by GCP

4. **Rotate source credentials regularly**
   - Even minimal-permission keys should be rotated
   - Consider using ADC or workload identity instead

5. **Monitor token generation**
   - Review Cloud Logging for `GenerateAccessToken` calls
   - Set up alerts for unusual patterns

## Comparison: Traditional vs. Short-lived

| Aspect | Traditional (Static Keys) | Short-lived Credentials |
|--------|--------------------------|------------------------|
| **Security** | Keys never expire | Tokens auto-expire (1-12h) |
| **Distribution** | Must share key files | No key distribution needed |
| **Rotation** | Manual, error-prone | Automatic per execution |
| **Blast Radius** | Full SA permissions until rotated | Limited to token lifetime |
| **Audit Trail** | Key usage only | Token generation + usage |
| **CI/CD Setup** | Full-access keys | Minimal impersonation keys |
| **Local Dev** | Shared key files | Personal ADC |
| **Compliance** | Challenging | Easier (short-lived tokens) |

## Migration Path

### Phase 1: Test (No Changes to Production)
```yaml
- provider: gcp
  id: test-short-lived
  use_short_lived_credentials: true
  service_account_email: "cloudlist@project.iam.gserviceaccount.com"
  gcp_service_account_key: "existing-key.json"  # Still using existing key
```

### Phase 2: Enable for Developers
```yaml
- provider: gcp
  id: dev-short-lived
  use_short_lived_credentials: true
  service_account_email: "cloudlist@project.iam.gserviceaccount.com"
  # No key - uses ADC from gcloud auth login
```

### Phase 3: Migrate CI/CD
```yaml
- provider: gcp
  id: ci-short-lived
  use_short_lived_credentials: true
  service_account_email: "powerful-sa@project.iam.gserviceaccount.com"
  source_credentials: "minimal-ci-sa.json"  # Minimal impersonation key
```

### Phase 4: Retire Static Keys
- Revoke old service account keys
- Remove `gcp_service_account_key` from configs
- Delete key files

## Additional Resources

- [GCP Short-lived Credentials Documentation](https://cloud.google.com/iam/docs/service-account-creds)
- [Application Default Credentials](https://cloud.google.com/docs/authentication/provide-credentials-adc)
- [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
- [IAM Best Practices](https://cloud.google.com/iam/docs/best-practices-service-accounts)
- [Service Account Impersonation](https://cloud.google.com/iam/docs/impersonating-service-accounts)
