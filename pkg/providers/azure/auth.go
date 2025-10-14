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
	certificatePath     = `certificate_path`     // Path to X.509 certificate file
	certificatePassword = `certificate_password` // Optional password for certificate
)

// createCredential creates an Azure credential based on the provider configuration.
// It supports multiple authentication methods with backward compatibility for Track 1 configs.
//
// Authentication priority order (following Azure SDK best practices):
// 1. Azure CLI (if use_cli_auth: true) - EXPLICIT, forces CLI-only (backward compatible)
// 2. Client Certificate (if certificate_path is provided) - EXPLICIT
// 3. Client Secret (if tenant_id, client_id, client_secret provided) - EXPLICIT (backward compatible)
// 4. DefaultAzureCredential (fallback) - AUTO-DETECTION chain:
//    a. Environment variables (AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, etc.)
//    b. Workload Identity (Kubernetes/GitHub Actions OIDC)
//    c. Managed Identity (Azure VMs, App Service, Container Apps, AKS)
//    d. Azure CLI (az login)
//    e. Azure PowerShell
//
// This follows industry standards where DefaultAzureCredential handles the credential chain automatically.
func createCredential(options schema.OptionBlock) (azcore.TokenCredential, error) {
	// Option 1: Azure CLI (BACKWARD COMPATIBLE - explicit CLI-only, skips managed identity, etc.)
	if UseCliAuth, _ := options.GetMetadata(useCliAuth); UseCliAuth == "true" {
		cred, err := azidentity.NewAzureCLICredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create AzureCLICredential: %w", err)
		}
		return cred, nil
	}

	// Option 2: Client Certificate (enterprise security) - EXPLICIT
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

	// Option 3: Client Secret (BACKWARD COMPATIBLE - explicit credentials)
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

	// Option 4: DefaultAzureCredential (FALLBACK - auto-detection, industry standard)
	// DefaultAzureCredential automatically tries credentials in this order:
	// 1. Environment variables (AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, etc.)
	// 2. Workload Identity (for Kubernetes/GitHub Actions with OIDC federation)
	// 3. Managed Identity (for Azure VMs, App Service, Container Apps, AKS pods)
	// 4. Azure CLI (az login session)
	// 5. Azure PowerShell (Azure PowerShell session)
	//
	// This is the recommended approach for production workloads as it works seamlessly across
	// different environments (local dev, CI/CD, Azure resources) without code changes.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DefaultAzureCredential: %w", err)
	}

	return cred, nil
}
