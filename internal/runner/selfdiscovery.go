package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	fileutil "github.com/projectdiscovery/utils/file"
)

// SelfDiscovery handles auto-discovery of cloud providers from environment variables
type SelfDiscovery struct {
	options *Options
}

// NewSelfDiscovery creates a new self-discovery instance
func NewSelfDiscovery(options *Options) *SelfDiscovery {
	return &SelfDiscovery{options: options}
}

// DiscoverProviders attempts to auto-discover available providers from environment
func (sd *SelfDiscovery) DiscoverProviders() schema.Options {
	discoveredConfig := schema.Options{}

	// Check which providers to discover
	providersToCheck := sd.getProvidersToCheck()

	for _, provider := range providersToCheck {
		switch provider {
		case "kubernetes":
			if config := sd.discoverKubernetes(); config != nil {
				discoveredConfig = append(discoveredConfig, config)
			}
		case "aws":
			if config := sd.discoverAWS(); config != nil {
				discoveredConfig = append(discoveredConfig, config)
			}
		case "gcp":
			if config := sd.discoverGCP(); config != nil {
				discoveredConfig = append(discoveredConfig, config)
			}
		case "azure":
			if config := sd.discoverAzure(); config != nil {
				discoveredConfig = append(discoveredConfig, config)
			}
		}
	}

	return discoveredConfig
}

// getProvidersToCheck returns the list of providers to check
func (sd *SelfDiscovery) getProvidersToCheck() []string {
	// If specific providers are requested, use those
	if len(sd.options.Providers) > 0 {
		return sd.options.Providers
	}

	// Otherwise, check all supported providers
	return []string{"kubernetes", "aws", "gcp", "azure"}
}

// discoverKubernetes attempts to discover Kubernetes configuration
func (sd *SelfDiscovery) discoverKubernetes() schema.OptionBlock {
	var kubeconfigPath string

	// Check KUBECONFIG environment variable first
	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		kubeconfigPath = envKubeconfig
		// KUBECONFIG can contain multiple paths separated by colon
		// Use the first one
		if strings.Contains(kubeconfigPath, ":") {
			kubeconfigPath = strings.Split(kubeconfigPath, ":")[0]
		}
	} else {
		// Fallback to default kubeconfig location
		homeDir, err := os.UserHomeDir()
		if err != nil {
			if sd.options.Verbose {
				gologger.Verbose().Msgf("Could not get user home directory: %s\n", err)
			}
			return nil
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", "config")
	}

	// check if /var/run/secrets/kubernetes.io/serviceaccount/token exists
	if fileutil.FileExists("/var/run/secrets/kubernetes.io/serviceaccount/token") {
		gologger.Info().Msgf("Discovered Kubernetes provider (in-cluster mode)\n")
		return schema.OptionBlock{
			"provider":       "kubernetes",
			"id":             "self-discovery",
			"incluster_mode": "true",
		}
	}

	// Check if kubeconfig file exists
	if !fileutil.FileExists(kubeconfigPath) {
		if sd.options.Verbose {
			gologger.Verbose().Msgf("Kubeconfig file not found at: %s\n", kubeconfigPath)
		}
		return nil
	}

	gologger.Info().Msgf("Discovered Kubernetes provider (kubeconfig: %s)\n", kubeconfigPath)

	return schema.OptionBlock{
		"provider":        "kubernetes",
		"id":              "self-discovery",
		"kubeconfig_file": kubeconfigPath,
	}
}

// discoverAWS attempts to discover AWS credentials
func (sd *SelfDiscovery) discoverAWS() schema.OptionBlock {
	// Priority 1: Check AWS IMDS (EC2/ECS instance metadata)
	if config := sd.tryAWSIMDS(); config != nil {
		return config
	}

	// Priority 2: Check ECS container credentials
	if config := sd.tryAWSECSCredentials(); config != nil {
		return config
	}

	// Priority 3: Check for AWS credentials in environment
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	// If we have access key and secret key, we can proceed
	if accessKey != "" && secretKey != "" {
		gologger.Info().Msgf("Discovered AWS provider (from environment variables)\n")

		config := schema.OptionBlock{
			"provider":       "aws",
			"id":             "self-discovery",
			"aws_access_key": accessKey,
			"aws_secret_key": secretKey,
		}

		if sessionToken != "" {
			config["aws_session_token"] = sessionToken
		}

		return config
	}

	// Priority 4: Check for AWS credentials file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	awsCredsPath := filepath.Join(homeDir, ".aws", "credentials")
	awsConfigPath := filepath.Join(homeDir, ".aws", "config")

	if fileutil.FileExists(awsCredsPath) || fileutil.FileExists(awsConfigPath) {
		if sd.options.Verbose {
			gologger.Verbose().Msgf("AWS credentials file found, but direct env variables not set\n")
			gologger.Verbose().Msgf("Please set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY or use provider config\n")
			profile := os.Getenv("AWS_PROFILE")
			if profile != "" {
				gologger.Verbose().Msgf("AWS_PROFILE detected: %s (not yet supported in self-discovery)\n", profile)
			}
		}
	}

	return nil
}

// tryAWSIMDS attempts to get credentials from AWS EC2 Instance Metadata Service
func (sd *SelfDiscovery) tryAWSIMDS() schema.OptionBlock {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}

	// Try IMDSv2 first (more secure, requires token)
	tokenReq, err := http.NewRequestWithContext(ctx, "PUT", "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return nil
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	tokenResp, err := client.Do(tokenReq)
	if err == nil && tokenResp.StatusCode == 200 {
		defer tokenResp.Body.Close()
		token, err := io.ReadAll(tokenResp.Body)
		if err == nil && len(token) > 0 {
			// We have IMDSv2 available
			if sd.hasValidIMDSRole(client, string(token)) {
				gologger.Info().Msgf("Discovered AWS provider (EC2 instance with IAM role - IMDSv2)\n")
				// AWS SDK will automatically use IMDS when no credentials are provided
				return schema.OptionBlock{
					"provider": "aws",
					"id":       "self-discovery",
					"use_imds": "true",
				}
			}
		}
	}

	// Fallback to IMDSv1
	roleReq, err := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/latest/meta-data/iam/security-credentials/", nil)
	if err != nil {
		return nil
	}

	roleResp, err := client.Do(roleReq)
	if err != nil {
		return nil
	}
	defer roleResp.Body.Close()

	if roleResp.StatusCode == 200 {
		gologger.Info().Msgf("Discovered AWS provider (EC2 instance with IAM role - IMDSv1)\n")
		// AWS SDK will automatically use IMDS when no credentials are provided
		return schema.OptionBlock{
			"provider": "aws",
			"id":       "self-discovery",
			"use_imds": "true",
		}
	}

	return nil
}

// hasValidIMDSRole checks if there's a valid IAM role attached
func (sd *SelfDiscovery) hasValidIMDSRole(client *http.Client, token string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/latest/meta-data/iam/security-credentials/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-aws-ec2-metadata-token", token)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	return resp.StatusCode == 200 && len(body) > 0
}

// tryAWSECSCredentials attempts to get credentials from ECS task role
func (sd *SelfDiscovery) tryAWSECSCredentials() schema.OptionBlock {
	// Check if running in ECS
	credentialsURI := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
	if credentialsURI == "" {
		// Also check for full URI (used in some configurations)
		credentialsURI = os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")
		if credentialsURI == "" {
			return nil
		}
	}

	gologger.Info().Msgf("Discovered AWS provider (ECS task with IAM role)\n")
	return schema.OptionBlock{
		"provider":          "aws",
		"id":                "self-discovery",
		"use_ecs_task_role": "true",
	}
}

// discoverGCP attempts to discover GCP credentials
func (sd *SelfDiscovery) discoverGCP() schema.OptionBlock {
	// Priority 1: Check GCP Metadata Service (GCE/GKE instance)
	if config := sd.tryGCPMetadataService(); config != nil {
		return config
	}

	// Priority 2: Check for GCP application credentials from environment
	credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	if credsPath == "" {
		// Check for default location
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		credsPath = filepath.Join(homeDir, ".config", "gcloud", "application_default_credentials.json")
	}

	if !fileutil.FileExists(credsPath) {
		if sd.options.Verbose {
			gologger.Verbose().Msgf("GCP credentials not found at: %s\n", credsPath)
		}
		return nil
	}

	// Read the service account key file
	keyData, err := os.ReadFile(credsPath)
	if err != nil {
		if sd.options.Verbose {
			gologger.Verbose().Msgf("Could not read GCP credentials file: %s\n", err)
		}
		return nil
	}

	gologger.Info().Msgf("Discovered GCP provider (credentials: %s)\n", credsPath)

	return schema.OptionBlock{
		"provider":                "gcp",
		"id":                      "self-discovery",
		"gcp_service_account_key": string(keyData),
	}
}

// tryGCPMetadataService attempts to get credentials from GCP Metadata Service
func (sd *SelfDiscovery) tryGCPMetadataService() schema.OptionBlock {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}

	// Try to access GCP metadata service
	req, err := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// Verify it's actually GCP by checking the response
		body, err := io.ReadAll(resp.Body)
		if err == nil && len(body) > 0 {
			// Try to parse as JSON to verify it's a valid token response
			var tokenData map[string]interface{}
			if json.Unmarshal(body, &tokenData) == nil {
				if _, ok := tokenData["access_token"]; ok {
					gologger.Info().Msgf("Discovered GCP provider (GCE/GKE instance with service account)\n")
					// Return empty service account key - GCP provider will use ADC (Application Default Credentials)
					// which automatically uses metadata service
					return schema.OptionBlock{
						"provider":                "gcp",
						"id":                      "self-discovery",
						"gcp_service_account_key": "",
					}
				}
			}
		}
	}

	return nil
}

// discoverAzure attempts to discover Azure credentials
func (sd *SelfDiscovery) discoverAzure() schema.OptionBlock {
	// Priority 1: Check Azure Managed Identity (MSI)
	if config := sd.tryAzureManagedIdentity(); config != nil {
		return config
	}

	// Priority 2: Check for Azure credentials in environment
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")

	if clientID != "" && clientSecret != "" && tenantID != "" && subscriptionID != "" {
		gologger.Info().Msgf("Discovered Azure provider (from environment variables)\n")

		return schema.OptionBlock{
			"provider":        "azure",
			"id":              "self-discovery",
			"client_id":       clientID,
			"client_secret":   clientSecret,
			"tenant_id":       tenantID,
			"subscription_id": subscriptionID,
		}
	}

	// Priority 3: Check if Azure CLI is configured and available
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	azureConfigPath := filepath.Join(homeDir, ".azure")
	if fileutil.FolderExists(azureConfigPath) {
		// Check if we have at least subscription ID
		if subscriptionID != "" {
			gologger.Info().Msgf("Discovered Azure provider (using Azure CLI auth)\n")

			return schema.OptionBlock{
				"provider":        "azure",
				"id":              "self-discovery",
				"subscription_id": subscriptionID,
				"use_cli_auth":    "true",
			}
		}

		if sd.options.Verbose {
			gologger.Verbose().Msgf("Azure CLI configuration found, but AZURE_SUBSCRIPTION_ID not set\n")
		}
	}

	return nil
}

// tryAzureManagedIdentity attempts to get credentials from Azure Managed Identity
func (sd *SelfDiscovery) tryAzureManagedIdentity() schema.OptionBlock {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}

	// Check for App Service / Azure Functions MSI endpoint (newer method)
	identityEndpoint := os.Getenv("IDENTITY_ENDPOINT")
	identityHeader := os.Getenv("IDENTITY_HEADER")

	if identityEndpoint != "" && identityHeader != "" {
		// App Service / Azure Functions
		req, err := http.NewRequestWithContext(ctx, "GET", identityEndpoint+"?api-version=2019-08-01&resource=https://management.azure.com/", nil)
		if err == nil {
			req.Header.Set("X-IDENTITY-HEADER", identityHeader)
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					gologger.Info().Msgf("Discovered Azure provider (App Service/Functions with Managed Identity)\n")
					// Get subscription ID from environment if available
					subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
					config := schema.OptionBlock{
						"provider": "azure",
						"id":       "self-discovery",
					}
					if subscriptionID != "" {
						config["subscription_id"] = subscriptionID
					}
					// Azure provider will use DefaultAzureCredential which includes managed identity
					return config
				}
			}
		}
	}

	// Check for older MSI endpoint (VM, Container Instances, etc.)
	msiEndpoint := os.Getenv("MSI_ENDPOINT")
	msiSecret := os.Getenv("MSI_SECRET")

	if msiEndpoint != "" && msiSecret != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", msiEndpoint+"?api-version=2017-09-01&resource=https://management.azure.com/", nil)
		if err == nil {
			req.Header.Set("Secret", msiSecret)
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					gologger.Info().Msgf("Discovered Azure provider (Container Instance with Managed Identity)\n")
					subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
					config := schema.OptionBlock{
						"provider": "azure",
						"id":       "self-discovery",
					}
					if subscriptionID != "" {
						config["subscription_id"] = subscriptionID
					}
					return config
				}
			}
		}
	}

	// Try standard Azure VM IMDS endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Metadata", "true")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err == nil && len(body) > 0 {
			// Verify it's a valid token response
			var tokenData map[string]interface{}
			if json.Unmarshal(body, &tokenData) == nil {
				if _, ok := tokenData["access_token"]; ok {
					gologger.Info().Msgf("Discovered Azure provider (VM/AKS with Managed Identity)\n")
					subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
					config := schema.OptionBlock{
						"provider": "azure",
						"id":       "self-discovery",
					}
					if subscriptionID != "" {
						config["subscription_id"] = subscriptionID
					}
					return config
				}
			}
		}
	}

	return nil
}
