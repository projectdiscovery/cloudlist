package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// staticWebAppsProvider is a provider for Azure Static Web Apps
type staticWebAppsProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (swp *staticWebAppsProvider) name() string {
	return "staticwebapps"
}

// GetResource returns all the Static Web Apps DNS endpoints for a provider.
func (swp *staticWebAppsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	staticSites, err := swp.fetchStaticSites(ctx)
	if err != nil {
		return nil, err
	}

	for _, site := range staticSites {
		// Extract default hostname
		if site.Properties != nil && site.Properties.DefaultHostname != nil {
			var metadata map[string]string
			if swp.extendedMetadata {
				metadata = swp.getStaticSiteMetadata(site)
			}

			resource := &schema.Resource{
				Provider: providerName,
				ID:       swp.id,
				DNSName:  *site.Properties.DefaultHostname,
				Service:  swp.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Extract custom domains if configured
		if site.Properties != nil && site.Properties.CustomDomains != nil {
			for _, customDomain := range site.Properties.CustomDomains {
				if customDomain != nil && *customDomain != "" {
					var metadata map[string]string
					if swp.extendedMetadata {
						metadata = swp.getStaticSiteMetadata(site)
						// Add indicator that this is a custom domain
						metadata["domain_type"] = "custom"
					}

					resource := &schema.Resource{
						Provider: providerName,
						ID:       swp.id,
						DNSName:  *customDomain,
						Service:  swp.name(),
						Metadata: metadata,
					}
					list.Append(resource)
				}
			}
		}
	}

	return list, nil
}

func (swp *staticWebAppsProvider) fetchStaticSites(ctx context.Context) ([]*armappservice.StaticSiteARMResource, error) {
	// Create static sites client
	client, err := armappservice.NewStaticSitesClient(swp.SubscriptionID, swp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create static sites client: %w", err)
	}

	// Use pager pattern to list all static sites
	var staticSites []*armappservice.StaticSiteARMResource
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list static sites: %w", err)
		}

		staticSites = append(staticSites, page.Value...)
	}

	return staticSites, nil
}

func (swp *staticWebAppsProvider) getStaticSiteMetadata(site *armappservice.StaticSiteARMResource) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "site_name", site.Name)
	schema.AddMetadata(metadata, "site_id", site.ID)
	metadata["subscription_id"] = swp.SubscriptionID
	metadata["owner_id"] = swp.SubscriptionID
	schema.AddMetadata(metadata, "location", site.Location)

	// Parse resource group from ID
	if site.ID != nil {
		_, resourceGroup := parseAzureResourceID(*site.ID)
		if resourceGroup != "" {
			metadata["resource_group"] = resourceGroup
		}
	}

	// SKU information
	if site.SKU != nil {
		schema.AddMetadata(metadata, "sku_name", site.SKU.Name)
		schema.AddMetadata(metadata, "sku_tier", site.SKU.Tier)
	}

	if site.Properties != nil {
		props := site.Properties

		// Default hostname
		schema.AddMetadata(metadata, "default_hostname", props.DefaultHostname)

		// Custom domains
		if len(props.CustomDomains) > 0 {
			var domains []string
			for _, domain := range props.CustomDomains {
				if domain != nil && *domain != "" {
					domains = append(domains, *domain)
				}
			}
			if len(domains) > 0 {
				metadata["custom_domains"] = strings.Join(domains, ",")
			}
		}

		// Repository information
		schema.AddMetadata(metadata, "repository_url", props.RepositoryURL)
		schema.AddMetadata(metadata, "branch", props.Branch)

		// Provider (GitHub, GitLab, etc.)
		if props.Provider != nil {
			metadata["provider"] = *props.Provider
		}

		// Build properties
		if props.BuildProperties != nil {
			schema.AddMetadata(metadata, "app_location", props.BuildProperties.AppLocation)
			schema.AddMetadata(metadata, "api_location", props.BuildProperties.APILocation)
			schema.AddMetadata(metadata, "output_location", props.BuildProperties.OutputLocation)
			schema.AddMetadata(metadata, "app_build_command", props.BuildProperties.AppBuildCommand)
			schema.AddMetadata(metadata, "api_build_command", props.BuildProperties.APIBuildCommand)

			// Combine build properties into a summary
			var buildProps []string
			if props.BuildProperties.AppLocation != nil {
				buildProps = append(buildProps, fmt.Sprintf("app:%s", *props.BuildProperties.AppLocation))
			}
			if props.BuildProperties.APILocation != nil {
				buildProps = append(buildProps, fmt.Sprintf("api:%s", *props.BuildProperties.APILocation))
			}
			if props.BuildProperties.OutputLocation != nil {
				buildProps = append(buildProps, fmt.Sprintf("output:%s", *props.BuildProperties.OutputLocation))
			}
			if len(buildProps) > 0 {
				metadata["build_properties"] = strings.Join(buildProps, ",")
			}
		}

		// Staging environment policy
		if props.StagingEnvironmentPolicy != nil {
			policy := string(*props.StagingEnvironmentPolicy)
			metadata["staging_environment_policy"] = policy
		}

		// Allow config file updates
		if props.AllowConfigFileUpdates != nil {
			metadata["allow_config_file_updates"] = fmt.Sprintf("%v", *props.AllowConfigFileUpdates)
		}

		// Content distribution endpoint
		schema.AddMetadata(metadata, "content_distribution_endpoint", props.ContentDistributionEndpoint)

		// Key vault reference identity
		schema.AddMetadata(metadata, "key_vault_reference_identity", props.KeyVaultReferenceIdentity)

		// User provided function apps
		if len(props.UserProvidedFunctionApps) > 0 {
			var functionApps []string
			for _, app := range props.UserProvidedFunctionApps {
				if app.ID != nil {
					functionApps = append(functionApps, *app.ID)
				}
			}
			if len(functionApps) > 0 {
				metadata["user_provided_function_apps"] = strings.Join(functionApps, ",")
			}
		}

		// Enterprise features
		if props.EnterpriseGradeCdnStatus != nil {
			status := string(*props.EnterpriseGradeCdnStatus)
			metadata["enterprise_cdn_status"] = status
		}
	}

	// Identity information
	if site.Identity != nil {
		if site.Identity.Type != nil {
			identityType := string(*site.Identity.Type)
			metadata["identity_type"] = identityType
		}
		schema.AddMetadata(metadata, "identity_principal_id", site.Identity.PrincipalID)
		schema.AddMetadata(metadata, "identity_tenant_id", site.Identity.TenantID)
	}

	// Tags
	if len(site.Tags) > 0 {
		if tagString := buildAzureTagString(site.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}
