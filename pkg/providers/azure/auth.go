package azure

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Configuration constants for Track 2 authentication
const (
	// Backward compatible (Track 1 config keys)
	// These are already defined in azure.go: id, tenantID, clientID, clientSecret, subscriptionID, useCliAuth

	// New Track 2 authentication options
	certificatePath       = `certificate_path`         // Path to X.509 certificate file
	certificatePassword   = `certificate_password`     // Optional password for certificate
	useWorkloadIdentity   = `use_workload_identity`   // Workload identity federation (K8s/GitHub Actions)
	useManagedIdentity    = `use_managed_identity`    // Explicit managed identity (system or user-assigned)
	managedIdentityID     = `managed_identity_id`     // Optional: user-assigned managed identity client ID
)

// createCredential creates an Azure credential based on the provider configuration.
// It supports multiple authentication methods with backward compatibility for Track 1 configs.
//
// Authentication priority order:
// 1. Azure CLI (if use_cli_auth: true) - BACKWARD COMPATIBLE, EXPLICIT
// 2. Workload Identity (if use_workload_identity: true) - EXPLICIT
// 3. Managed Identity (if use_managed_identity: true) - EXPLICIT
// 4. Client Certificate (if certificate_path is provided) - EXPLICIT
// 5. Client Secret (if tenant_id, client_id, client_secret provided) - BACKWARD COMPATIBLE, EXPLICIT
// 6. DefaultAzureCredential (fallback when no explicit auth specified) - AUTO-DETECTION
func createCredential(options schema.OptionBlock) (azcore.TokenCredential, error) {
	// Option 1: Azure CLI (BACKWARD COMPATIBLE - explicit, only tries CLI)
	if UseCliAuth, _ := options.GetMetadata(useCliAuth); UseCliAuth == "true" {
		cred, err := azidentity.NewAzureCLICredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create AzureCLICredential: %w", err)
		}
		return cred, nil
	}

	// Option 2: Workload Identity (Kubernetes, GitHub Actions OIDC) - EXPLICIT
	if useWorkload, _ := options.GetMetadata(useWorkloadIdentity); useWorkload == "true" {
		cred, err := azidentity.NewWorkloadIdentityCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create WorkloadIdentityCredential: %w", err)
		}
		return cred, nil
	}

	// Option 3: Managed Identity (Azure VMs, App Service, Container Apps, AKS) - EXPLICIT
	if useManaged, _ := options.GetMetadata(useManagedIdentity); useManaged == "true" {
		opts := &azidentity.ManagedIdentityCredentialOptions{}

		// Support user-assigned managed identity
		if clientID, ok := options.GetMetadata(managedIdentityID); ok {
			opts.ID = azidentity.ClientID(clientID)
		}

		cred, err := azidentity.NewManagedIdentityCredential(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create ManagedIdentityCredential: %w", err)
		}
		return cred, nil
	}

	// Option 4: Client Certificate (enterprise security) - EXPLICIT
	if certPath, ok := options.GetMetadata(certificatePath); ok {
		TenantID, ok := options.GetMetadata(tenantID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: tenantID}
		}
		ClientID, ok := options.GetMetadata(clientID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: clientID}
		}

		// Read certificate from file
		certData, err := os.ReadFile(certPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}

		opts := &azidentity.ClientCertificateCredentialOptions{}

		// Handle password-protected certificates
		if certPass, ok := options.GetMetadata(certificatePassword); ok {
			certs, key, err := azidentity.ParseCertificates(certData, []byte(certPass))
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate with password: %w", err)
			}
			cred, err := azidentity.NewClientCertificateCredential(TenantID, ClientID, certs, key, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to create ClientCertificateCredential: %w", err)
			}
			return cred, nil
		}

		// Parse unprotected certificate
		certs, key, err := azidentity.ParseCertificates(certData, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate: %w", err)
		}

		cred, err := azidentity.NewClientCertificateCredential(TenantID, ClientID, certs, key, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create ClientCertificateCredential: %w", err)
		}
		return cred, nil
	}

	// Option 5: Client Secret (BACKWARD COMPATIBLE - explicit credentials)
	ClientID, hasClientID := options.GetMetadata(clientID)
	if hasClientID {
		ClientSecret, ok := options.GetMetadata(clientSecret)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: clientSecret}
		}

		TenantID, ok := options.GetMetadata(tenantID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: tenantID}
		}

		cred, err := azidentity.NewClientSecretCredential(TenantID, ClientID, ClientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create ClientSecretCredential: %w", err)
		}

		return cred, nil
	}

	// Option 6: DefaultAzureCredential (FALLBACK - no explicit auth specified)
	// This auto-detects: env vars → workload identity → managed identity → Azure CLI
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DefaultAzureCredential: %w", err)
	}

	return cred, nil
}
