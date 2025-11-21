package runner

import (
	"os"
	"path/filepath"
	"strings"

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
	// Check for AWS credentials in environment
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	// Also check for AWS profile
	profile := os.Getenv("AWS_PROFILE")

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

	// Check for AWS credentials file
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
			if profile != "" {
				gologger.Verbose().Msgf("AWS_PROFILE detected: %s (not yet supported in self-discovery)\n", profile)
			}
		}
	}

	return nil
}

// discoverGCP attempts to discover GCP credentials
func (sd *SelfDiscovery) discoverGCP() schema.OptionBlock {
	// Check for GCP application credentials
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

// discoverAzure attempts to discover Azure credentials
func (sd *SelfDiscovery) discoverAzure() schema.OptionBlock {
	// Check for Azure credentials in environment
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

	// Check if Azure CLI is configured and available
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
