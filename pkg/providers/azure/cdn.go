package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cdn/armcdn"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// cdnProvider is a provider for Azure CDN Profiles and Endpoints
type cdnProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential // Track 2: replaced autorest.Authorizer
	extendedMetadata bool
}

// name returns the name of the provider
func (cdn *cdnProvider) name() string {
	return "cdn"
}

// GetResource returns all the CDN endpoints for a provider.
func (cdn *cdnProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	profiles, err := cdn.fetchCDNProfiles(ctx)
	if err != nil {
		return nil, err
	}

	for _, profile := range profiles {
		if profile.Name == nil {
			continue
		}

		// Parse resource group from profile ID
		_, resourceGroup := parseAzureResourceID(*profile.ID)
		if resourceGroup == "" {
			gologger.Warning().Msgf("Failed to parse resource group from profile ID: %s", *profile.ID)
			continue
		}

		// Fetch endpoints for this profile
		endpoints, err := cdn.fetchCDNEndpoints(ctx, resourceGroup, *profile.Name)
		if err != nil {
			gologger.Warning().Msgf("Error fetching endpoints for CDN profile %s: %s", *profile.Name, err)
			continue
		}

		for _, endpoint := range endpoints {
			if endpoint.Properties == nil || endpoint.Properties.HostName == nil {
				continue
			}

			var metadata map[string]string
			if cdn.extendedMetadata {
				metadata = cdn.getCDNMetadata(profile, endpoint)
			}

			// Create resource for primary endpoint hostname
			resource := &schema.Resource{
				Provider: providerName,
				ID:       cdn.id,
				DNSName:  *endpoint.Properties.HostName,
				Service:  cdn.name(),
				Metadata: metadata,
			}
			list.Append(resource)

			// Add custom domains if present
			if endpoint.Properties.CustomDomains != nil {
				for _, customDomain := range endpoint.Properties.CustomDomains {
					if customDomain.Properties != nil && customDomain.Properties.HostName != nil {
						customDomainResource := &schema.Resource{
							Provider: providerName,
							ID:       cdn.id,
							DNSName:  *customDomain.Properties.HostName,
							Service:  cdn.name(),
						}
						if metadata != nil {
							customDomainResource.Metadata = make(map[string]string)
							for k, v := range metadata {
								customDomainResource.Metadata[k] = v
							}
							customDomainResource.Metadata["is_custom_domain"] = "true"
						}
						list.Append(customDomainResource)
					}
				}
			}
		}
	}

	return list, nil
}

func (cdn *cdnProvider) fetchCDNProfiles(ctx context.Context) ([]*armcdn.Profile, error) {
	// Track 2: Create profiles client directly
	client, err := armcdn.NewProfilesClient(cdn.SubscriptionID, cdn.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create CDN profiles client: %w", err)
	}

	// Track 2: Use pager pattern
	var profiles []*armcdn.Profile
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list CDN profiles: %w", err)
		}

		profiles = append(profiles, page.Value...)
	}

	return profiles, nil
}

func (cdn *cdnProvider) fetchCDNEndpoints(ctx context.Context, resourceGroup, profileName string) ([]*armcdn.Endpoint, error) {
	// Track 2: Create endpoints client directly
	client, err := armcdn.NewEndpointsClient(cdn.SubscriptionID, cdn.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create CDN endpoints client: %w", err)
	}

	// Track 2: Use pager pattern
	var endpoints []*armcdn.Endpoint
	pager := client.NewListByProfilePager(resourceGroup, profileName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list CDN endpoints: %w", err)
		}

		endpoints = append(endpoints, page.Value...)
	}

	return endpoints, nil
}

func (cdn *cdnProvider) getCDNMetadata(profile *armcdn.Profile, endpoint *armcdn.Endpoint) map[string]string {
	metadata := make(map[string]string)

	// Profile metadata
	schema.AddMetadata(metadata, "profile_name", profile.Name)
	schema.AddMetadata(metadata, "profile_id", profile.ID)
	schema.AddMetadata(metadata, "location", profile.Location)
	metadata["subscription_id"] = cdn.SubscriptionID
	metadata["owner_id"] = cdn.SubscriptionID

	// Track 2: Parse resource group from ID manually
	if profile.ID != nil {
		parts := strings.Split(*profile.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	// Profile SKU
	if profile.SKU != nil && profile.SKU.Name != nil {
		skuName := string(*profile.SKU.Name)
		metadata["sku"] = skuName
	}

	// Profile properties
	if profile.Properties != nil {
		if profile.Properties.ProvisioningState != nil {
			provisioningState := string(*profile.Properties.ProvisioningState)
			schema.AddMetadata(metadata, "profile_provisioning_state", &provisioningState)
		}
		if profile.Properties.ResourceState != nil {
			resourceState := string(*profile.Properties.ResourceState)
			metadata["profile_resource_state"] = resourceState
		}
	}

	// Endpoint metadata
	schema.AddMetadata(metadata, "endpoint_name", endpoint.Name)
	schema.AddMetadata(metadata, "endpoint_id", endpoint.ID)

	if endpoint.Properties != nil {
		props := endpoint.Properties

		if props.ProvisioningState != nil {
			provisioningState := string(*props.ProvisioningState)
			schema.AddMetadata(metadata, "endpoint_provisioning_state", &provisioningState)
		}

		if props.ResourceState != nil {
			resourceState := string(*props.ResourceState)
			metadata["endpoint_resource_state"] = resourceState
		}

		if props.OriginHostHeader != nil {
			metadata["origin_host_header"] = *props.OriginHostHeader
		}

		if props.OptimizationType != nil {
			optimizationType := string(*props.OptimizationType)
			metadata["optimization_type"] = optimizationType
		}

		if props.IsHTTPAllowed != nil {
			metadata["is_http_allowed"] = fmt.Sprintf("%v", *props.IsHTTPAllowed)
		}

		if props.IsHTTPSAllowed != nil {
			metadata["is_https_allowed"] = fmt.Sprintf("%v", *props.IsHTTPSAllowed)
		}

		if props.IsCompressionEnabled != nil {
			metadata["is_compression_enabled"] = fmt.Sprintf("%v", *props.IsCompressionEnabled)
		}

		if props.ProbePath != nil {
			metadata["probe_path"] = *props.ProbePath
		}

		if props.QueryStringCachingBehavior != nil {
			queryStringBehavior := string(*props.QueryStringCachingBehavior)
			metadata["query_string_caching_behavior"] = queryStringBehavior
		}

		// Origin information
		if len(props.Origins) > 0 {
			var originHosts []string
			for _, origin := range props.Origins {
				if origin.Properties != nil && origin.Properties.HostName != nil {
					originHosts = append(originHosts, *origin.Properties.HostName)
				}
			}
			if len(originHosts) > 0 {
				metadata["origin_hosts"] = strings.Join(originHosts, ",")
			}
		}

		// Custom domain count
		if props.CustomDomains != nil {
			metadata["custom_domains_count"] = fmt.Sprintf("%d", len(props.CustomDomains))
		}

		// Delivery policies
		if props.DeliveryPolicy != nil && props.DeliveryPolicy.Rules != nil {
			metadata["delivery_policy_rules_count"] = fmt.Sprintf("%d", len(props.DeliveryPolicy.Rules))
		}

		// Geo filters
		if props.GeoFilters != nil {
			metadata["geo_filters_count"] = fmt.Sprintf("%d", len(props.GeoFilters))
		}

		// Content types to compress
		if len(props.ContentTypesToCompress) > 0 {
			var contentTypes []string
			for _, ct := range props.ContentTypesToCompress {
				if ct != nil {
					contentTypes = append(contentTypes, *ct)
				}
			}
			if len(contentTypes) > 0 {
				metadata["content_types_to_compress"] = strings.Join(contentTypes, ",")
			}
		}
	}

	// Profile tags
	if len(profile.Tags) > 0 {
		if tagString := buildAzureTagString(profile.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}
