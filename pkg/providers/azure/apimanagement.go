package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/apimanagement/armapimanagement"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// apiManagementProvider is a provider for Azure API Management services
type apiManagementProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (amp *apiManagementProvider) name() string {
	return "apimanagement"
}

// GetResource returns all the API Management resources for a provider.
func (amp *apiManagementProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	services, err := amp.fetchAPIManagementServices(ctx)
	if err != nil {
		return nil, err
	}

	for _, service := range services {
		if service.Properties == nil {
			continue
		}

		var metadata map[string]string
		if amp.extendedMetadata {
			metadata = amp.getAPIManagementMetadata(service)
		}

		// Extract gateway URL (primary DNS endpoint)
		if service.Properties.GatewayURL != nil {
			gatewayDNS := extractDNSFromURL(*service.Properties.GatewayURL)
			if gatewayDNS != "" {
				resource := &schema.Resource{
					Provider: providerName,
					ID:       amp.id,
					DNSName:  gatewayDNS,
					Service:  amp.name(),
					Metadata: metadata,
				}
				list.Append(resource)
			}
		}

		// Extract portal URL
		if service.Properties.PortalURL != nil {
			portalDNS := extractDNSFromURL(*service.Properties.PortalURL)
			if portalDNS != "" {
				resource := &schema.Resource{
					Provider: providerName,
					ID:       amp.id,
					DNSName:  portalDNS,
					Service:  amp.name(),
				}
				if metadata != nil {
					resource.Metadata = make(map[string]string)
					for k, v := range metadata {
						resource.Metadata[k] = v
					}
				}
				list.Append(resource)
			}
		}

		// Extract management API URL
		if service.Properties.ManagementAPIURL != nil {
			managementDNS := extractDNSFromURL(*service.Properties.ManagementAPIURL)
			if managementDNS != "" {
				resource := &schema.Resource{
					Provider: providerName,
					ID:       amp.id,
					DNSName:  managementDNS,
					Service:  amp.name(),
				}
				if metadata != nil {
					resource.Metadata = make(map[string]string)
					for k, v := range metadata {
						resource.Metadata[k] = v
					}
				}
				list.Append(resource)
			}
		}

		// Extract custom domains if configured
		if service.Properties.HostnameConfigurations != nil {
			for _, hostnameConfig := range service.Properties.HostnameConfigurations {
				if hostnameConfig.HostName != nil {
					resource := &schema.Resource{
						Provider: providerName,
						ID:       amp.id,
						DNSName:  *hostnameConfig.HostName,
						Service:  amp.name(),
					}
					if metadata != nil {
						resource.Metadata = make(map[string]string)
						for k, v := range metadata {
							resource.Metadata[k] = v
						}
						if hostnameConfig.Type != nil {
							resource.Metadata["hostname_type"] = string(*hostnameConfig.Type)
						}
					}
					list.Append(resource)
				}
			}
		}

		// Extract public IP addresses
		if service.Properties.PublicIPAddresses != nil {
			for _, ip := range service.Properties.PublicIPAddresses {
				if ip != nil {
					resource := &schema.Resource{
						Provider:   providerName,
						ID:         amp.id,
						PublicIPv4: *ip,
						Service:    amp.name(),
					}
					if metadata != nil {
						resource.Metadata = make(map[string]string)
						for k, v := range metadata {
							resource.Metadata[k] = v
						}
					}
					list.Append(resource)
				}
			}
		}

		// Extract private IP addresses (VNet integration)
		if service.Properties.PrivateIPAddresses != nil {
			for _, ip := range service.Properties.PrivateIPAddresses {
				if ip != nil {
					resource := &schema.Resource{
						Provider:    providerName,
						ID:          amp.id,
						PrivateIpv4: *ip,
						Service:     amp.name(),
					}
					if metadata != nil {
						resource.Metadata = make(map[string]string)
						for k, v := range metadata {
							resource.Metadata[k] = v
						}
					}
					list.Append(resource)
				}
			}
		}
	}

	return list, nil
}

func (amp *apiManagementProvider) fetchAPIManagementServices(ctx context.Context) ([]*armapimanagement.ServiceResource, error) {
	// Track 2: Create API Management service client
	client, err := armapimanagement.NewServiceClient(amp.SubscriptionID, amp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create API Management service client: %w", err)
	}

	// Track 2: Use pager pattern
	var services []*armapimanagement.ServiceResource
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list API Management services: %w", err)
		}

		services = append(services, page.Value...)
	}

	return services, nil
}

func (amp *apiManagementProvider) getAPIManagementMetadata(service *armapimanagement.ServiceResource) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "apim_name", service.Name)
	schema.AddMetadata(metadata, "apim_id", service.ID)
	metadata["subscription_id"] = amp.SubscriptionID
	metadata["owner_id"] = amp.SubscriptionID
	schema.AddMetadata(metadata, "location", service.Location)
	schema.AddMetadata(metadata, "type", service.Type)

	// Parse resource group from ID
	if service.ID != nil {
		parts := strings.Split(*service.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if service.Properties != nil {
		props := service.Properties

		if props.ProvisioningState != nil {
			metadata["provisioning_state"] = *props.ProvisioningState
		}

		schema.AddMetadata(metadata, "publisher_email", props.PublisherEmail)
		schema.AddMetadata(metadata, "publisher_name", props.PublisherName)

		schema.AddMetadata(metadata, "gateway_url", props.GatewayURL)
		schema.AddMetadata(metadata, "portal_url", props.PortalURL)
		schema.AddMetadata(metadata, "management_api_url", props.ManagementAPIURL)

		if props.GatewayRegionalURL != nil {
			metadata["gateway_regional_url"] = *props.GatewayRegionalURL
		}

		if props.VirtualNetworkType != nil {
			metadata["virtual_network_type"] = string(*props.VirtualNetworkType)
		}

		if len(props.PublicIPAddresses) > 0 {
			var ips []string
			for _, ip := range props.PublicIPAddresses {
				if ip != nil {
					ips = append(ips, *ip)
				}
			}
			if len(ips) > 0 {
				metadata["public_ip_addresses"] = strings.Join(ips, ",")
			}
		}

		if len(props.PrivateIPAddresses) > 0 {
			var ips []string
			for _, ip := range props.PrivateIPAddresses {
				if ip != nil {
					ips = append(ips, *ip)
				}
			}
			if len(ips) > 0 {
				metadata["private_ip_addresses"] = strings.Join(ips, ",")
			}
		}

		if props.TargetProvisioningState != nil {
			metadata["target_provisioning_state"] = *props.TargetProvisioningState
		}

		if props.CreatedAtUTC != nil {
			metadata["created_at"] = props.CreatedAtUTC.String()
		}

		if props.DeveloperPortalURL != nil {
			metadata["developer_portal_url"] = *props.DeveloperPortalURL
		}

		if len(props.HostnameConfigurations) > 0 {
			var hostnames []string
			for _, config := range props.HostnameConfigurations {
				if config.HostName != nil {
					hostType := "unknown"
					if config.Type != nil {
						hostType = string(*config.Type)
					}
					hostnames = append(hostnames, fmt.Sprintf("%s:%s", hostType, *config.HostName))
				}
			}
			if len(hostnames) > 0 {
				metadata["custom_hostnames"] = strings.Join(hostnames, ",")
			}
		}

		if props.NotificationSenderEmail != nil {
			metadata["notification_sender_email"] = *props.NotificationSenderEmail
		}

		if props.PlatformVersion != nil {
			metadata["platform_version"] = string(*props.PlatformVersion)
		}

		if props.DisableGateway != nil {
			metadata["disable_gateway"] = fmt.Sprintf("%v", *props.DisableGateway)
		}

		if props.EnableClientCertificate != nil {
			metadata["enable_client_certificate"] = fmt.Sprintf("%v", *props.EnableClientCertificate)
		}

		if props.Restore != nil {
			metadata["restore"] = fmt.Sprintf("%v", *props.Restore)
		}
	}

	// SKU information
	if service.SKU != nil {
		if service.SKU.Name != nil {
			metadata["sku_name"] = string(*service.SKU.Name)
		}
		if service.SKU.Capacity != nil {
			metadata["sku_capacity"] = fmt.Sprintf("%d", *service.SKU.Capacity)
		}
	}

	// Zones
	if len(service.Zones) > 0 {
		var zones []string
		for _, z := range service.Zones {
			if z != nil {
				zones = append(zones, *z)
			}
		}
		if len(zones) > 0 {
			metadata["availability_zones"] = strings.Join(zones, ",")
		}
	}

	// Tags
	if len(service.Tags) > 0 {
		if tagString := buildAzureTagString(service.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// Identity
	if service.Identity != nil && service.Identity.Type != nil {
		metadata["identity_type"] = string(*service.Identity.Type)
		if service.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *service.Identity.PrincipalID
		}
		if service.Identity.TenantID != nil {
			metadata["identity_tenant_id"] = *service.Identity.TenantID
		}
	}

	return metadata
}

// extractDNSFromURL extracts the DNS hostname from a URL
func extractDNSFromURL(url string) string {
	// Remove protocol prefix (https://, http://)
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove path and query string
	if idx := strings.Index(url, "/"); idx > 0 {
		url = url[:idx]
	}

	// Remove port
	if idx := strings.Index(url, ":"); idx > 0 {
		url = url[:idx]
	}

	return url
}
