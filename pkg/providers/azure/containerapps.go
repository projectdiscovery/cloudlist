package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// containerAppsProvider is a provider for Azure Container Apps
type containerAppsProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (cap *containerAppsProvider) name() string {
	return "containerapps"
}

// GetResource returns all the Container Apps resources for a provider.
func (cap *containerAppsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	containerApps, err := cap.fetchContainerApps(ctx)
	if err != nil {
		return nil, err
	}

	for _, app := range containerApps {
		if app.Properties == nil {
			continue
		}

		var metadata map[string]string
		if cap.extendedMetadata {
			metadata = cap.getContainerAppMetadata(app)
		}

		// Extract FQDN from ingress configuration
		if app.Properties.Configuration != nil && app.Properties.Configuration.Ingress != nil && app.Properties.Configuration.Ingress.Fqdn != nil {
			resource := &schema.Resource{
				Provider: providerName,
				ID:       cap.id,
				DNSName:  *app.Properties.Configuration.Ingress.Fqdn,
				Service:  cap.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Extract custom domains if configured
		if app.Properties.Configuration != nil && app.Properties.Configuration.Ingress != nil && app.Properties.Configuration.Ingress.CustomDomains != nil {
			for _, customDomain := range app.Properties.Configuration.Ingress.CustomDomains {
				if customDomain.Name != nil {
					customResource := &schema.Resource{
						Provider: providerName,
						ID:       cap.id,
						DNSName:  *customDomain.Name,
						Service:  cap.name(),
					}
					if metadata != nil {
						customResource.Metadata = make(map[string]string)
						for k, v := range metadata {
							customResource.Metadata[k] = v
						}
						customResource.Metadata["is_custom_domain"] = "true"
					}
					list.Append(customResource)
				}
			}
		}

		// Extract outbound IPs (public)
		if len(app.Properties.OutboundIPAddresses) > 0 {
			for _, ip := range app.Properties.OutboundIPAddresses {
				if ip != nil {
					ipResource := &schema.Resource{
						Provider:    providerName,
						ID:          cap.id,
						PublicIPv4:  *ip,
						Service:     cap.name(),
						Public:      true,
					}
					if metadata != nil {
						ipResource.Metadata = make(map[string]string)
						for k, v := range metadata {
							ipResource.Metadata[k] = v
						}
						ipResource.Metadata["ip_type"] = "outbound"
					}
					list.Append(ipResource)
				}
			}
		}

		// Extract latest revision FQDN if available
		if app.Properties.LatestRevisionFqdn != nil && *app.Properties.LatestRevisionFqdn != "" {
			revisionResource := &schema.Resource{
				Provider: providerName,
				ID:       cap.id,
				DNSName:  *app.Properties.LatestRevisionFqdn,
				Service:  cap.name(),
			}
			if metadata != nil {
				revisionResource.Metadata = make(map[string]string)
				for k, v := range metadata {
					revisionResource.Metadata[k] = v
				}
				revisionResource.Metadata["is_revision_fqdn"] = "true"
			}
			list.Append(revisionResource)
		}
	}

	return list, nil
}

func (cap *containerAppsProvider) fetchContainerApps(ctx context.Context) ([]*armappcontainers.ContainerApp, error) {
	// Track 2: Create container apps client directly
	client, err := armappcontainers.NewContainerAppsClient(cap.SubscriptionID, cap.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container apps client: %w", err)
	}

	// Track 2: Use pager pattern to list all container apps in subscription
	var containerApps []*armappcontainers.ContainerApp
	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list container apps: %w", err)
		}

		containerApps = append(containerApps, page.Value...)
	}

	return containerApps, nil
}

func (cap *containerAppsProvider) getContainerAppMetadata(app *armappcontainers.ContainerApp) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "app_name", app.Name)
	schema.AddMetadata(metadata, "app_id", app.ID)
	metadata["subscription_id"] = cap.SubscriptionID
	metadata["owner_id"] = cap.SubscriptionID
	schema.AddMetadata(metadata, "location", app.Location)

	// Parse resource group from ID manually
	if app.ID != nil {
		parts := strings.Split(*app.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if app.Properties != nil {
		props := app.Properties

		if props.ProvisioningState != nil {
			provisioningState := string(*props.ProvisioningState)
			metadata["provisioning_state"] = provisioningState
		}

		schema.AddMetadata(metadata, "managed_environment_id", props.ManagedEnvironmentID)

		// Ingress configuration
		if props.Configuration != nil && props.Configuration.Ingress != nil {
			ingress := props.Configuration.Ingress

			if ingress.External != nil {
				metadata["ingress_external"] = fmt.Sprintf("%v", *ingress.External)
			}

			schema.AddMetadata(metadata, "ingress_fqdn", ingress.Fqdn)

			if ingress.TargetPort != nil {
				metadata["ingress_target_port"] = fmt.Sprintf("%d", *ingress.TargetPort)
			}

			if ingress.Transport != nil {
				transport := string(*ingress.Transport)
				metadata["ingress_transport"] = transport
			}

			if len(ingress.Traffic) > 0 {
				metadata["traffic_weights_count"] = fmt.Sprintf("%d", len(ingress.Traffic))
			}

			if ingress.AllowInsecure != nil {
				metadata["ingress_allow_insecure"] = fmt.Sprintf("%v", *ingress.AllowInsecure)
			}

			if len(ingress.CustomDomains) > 0 {
				var customDomainNames []string
				for _, domain := range ingress.CustomDomains {
					if domain.Name != nil {
						customDomainNames = append(customDomainNames, *domain.Name)
					}
				}
				if len(customDomainNames) > 0 {
					metadata["custom_domains"] = strings.Join(customDomainNames, ",")
				}
			}
		}

		// Latest revision information
		schema.AddMetadata(metadata, "latest_revision_name", props.LatestRevisionName)
		schema.AddMetadata(metadata, "latest_revision_fqdn", props.LatestRevisionFqdn)

		// Outbound IP addresses
		if len(props.OutboundIPAddresses) > 0 {
			var outboundIPs []string
			for _, ip := range props.OutboundIPAddresses {
				if ip != nil {
					outboundIPs = append(outboundIPs, *ip)
				}
			}
			if len(outboundIPs) > 0 {
				metadata["outbound_ip_addresses"] = strings.Join(outboundIPs, ",")
			}
		}
	}

	// Tags
	if len(app.Tags) > 0 {
		if tagString := buildAzureTagString(app.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// Identity
	if app.Identity != nil && app.Identity.Type != nil {
		identityType := string(*app.Identity.Type)
		schema.AddMetadata(metadata, "identity_type", &identityType)

		if app.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *app.Identity.PrincipalID
		}

		if app.Identity.TenantID != nil {
			metadata["identity_tenant_id"] = *app.Identity.TenantID
		}
	}

	return metadata
}
