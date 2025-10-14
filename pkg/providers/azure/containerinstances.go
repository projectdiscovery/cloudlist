package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// containerInstancesProvider is a provider for Azure Container Instances
type containerInstancesProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (cip *containerInstancesProvider) name() string {
	return "containerinstances"
}

// GetResource returns all the Container Instances resources for a provider.
func (cip *containerInstancesProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	containerGroups, err := cip.fetchContainerGroups(ctx)
	if err != nil {
		return nil, err
	}

	for _, cg := range containerGroups {
		if cg.Properties == nil {
			continue
		}

		var metadata map[string]string
		if cip.extendedMetadata {
			metadata = cip.getContainerGroupMetadata(cg)
		}

		// Extract public IP and FQDN from IPAddress
		if cg.Properties.IPAddress != nil {
			// Public IP resource - only add if not explicitly marked as private
			if cg.Properties.IPAddress.IP != nil {
				// Check if this is a private IP to avoid misclassification
				isPrivate := cg.Properties.IPAddress.Type != nil && *cg.Properties.IPAddress.Type == armcontainerinstance.ContainerGroupIPAddressTypePrivate

				if !isPrivate {
					resource := &schema.Resource{
						Provider:   providerName,
						ID:         cip.id,
						Public:     true,
						Service:    cip.name(),
						PublicIPv4: *cg.Properties.IPAddress.IP,
						Metadata:   metadata,
					}
					list.Append(resource)
				}
			}

			// FQDN resource
			if cg.Properties.IPAddress.Fqdn != nil {
				dnsResource := &schema.Resource{
					Provider: providerName,
					ID:       cip.id,
					DNSName:  *cg.Properties.IPAddress.Fqdn,
					Service:  cip.name(),
				}
				if metadata != nil {
					dnsResource.Metadata = make(map[string]string)
					for k, v := range metadata {
						dnsResource.Metadata[k] = v
					}
				}
				list.Append(dnsResource)
			}
		}

		// Extract private IPs from subnet IDs (VNet integration)
		if len(cg.Properties.SubnetIDs) > 0 {
			// Container groups with VNet integration have private IPs
			// The private IP is available in IPAddress even for VNet-integrated groups
			if cg.Properties.IPAddress != nil && cg.Properties.IPAddress.IP != nil {
				// Check if this is a private IP (VNet-integrated)
				if cg.Properties.IPAddress.Type != nil && *cg.Properties.IPAddress.Type == armcontainerinstance.ContainerGroupIPAddressTypePrivate {
					resource := &schema.Resource{
						Provider:    providerName,
						ID:          cip.id,
						Public:      false,
						Service:     cip.name(),
						PrivateIpv4: *cg.Properties.IPAddress.IP,
						Metadata:    metadata,
					}
					list.Append(resource)
				}
			}
		}
	}

	return list, nil
}

func (cip *containerInstancesProvider) fetchContainerGroups(ctx context.Context) ([]*armcontainerinstance.ContainerGroup, error) {
	// Track 2: Create container groups client directly
	client, err := armcontainerinstance.NewContainerGroupsClient(cip.SubscriptionID, cip.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container groups client: %w", err)
	}

	// Track 2: Use pager pattern
	var containerGroups []*armcontainerinstance.ContainerGroup
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list container groups: %w", err)
		}

		containerGroups = append(containerGroups, page.Value...)
	}

	return containerGroups, nil
}

func (cip *containerInstancesProvider) getContainerGroupMetadata(cg *armcontainerinstance.ContainerGroup) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "container_group_name", cg.Name)
	schema.AddMetadata(metadata, "container_group_id", cg.ID)
	metadata["subscription_id"] = cip.SubscriptionID
	metadata["owner_id"] = cip.SubscriptionID
	schema.AddMetadata(metadata, "location", cg.Location)
	schema.AddMetadata(metadata, "type", cg.Type)

	// Parse resource group from ID manually
	if cg.ID != nil {
		_, resourceGroup := parseAzureResourceID(*cg.ID)
		if resourceGroup != "" {
			metadata["resource_group"] = resourceGroup
		}
	}

	if cg.Properties != nil {
		props := cg.Properties

		// OS Type
		if props.OSType != nil {
			osType := string(*props.OSType)
			metadata["os_type"] = osType
		}

		// Restart Policy
		if props.RestartPolicy != nil {
			restartPolicy := string(*props.RestartPolicy)
			metadata["restart_policy"] = restartPolicy
		}

		// SKU
		if props.SKU != nil {
			sku := string(*props.SKU)
			metadata["sku"] = sku
		}

		// Provisioning State
		if props.ProvisioningState != nil {
			metadata["provisioning_state"] = *props.ProvisioningState
		}

		// Instance View State
		if props.InstanceView != nil && props.InstanceView.State != nil {
			metadata["state"] = *props.InstanceView.State
		}

		// IP Address Information
		if props.IPAddress != nil {
			schema.AddMetadata(metadata, "ip_address", props.IPAddress.IP)
			schema.AddMetadata(metadata, "fqdn", props.IPAddress.Fqdn)

			if props.IPAddress.Type != nil {
				ipAddressType := string(*props.IPAddress.Type)
				metadata["ip_address_type"] = ipAddressType
			}

			// DNS Name Label
			schema.AddMetadata(metadata, "dns_name_label", props.IPAddress.DNSNameLabel)

			// Ports
			if len(props.IPAddress.Ports) > 0 {
				var ports []string
				for _, port := range props.IPAddress.Ports {
					if port.Port != nil {
						protocol := "TCP"
						if port.Protocol != nil {
							protocol = string(*port.Protocol)
						}
						ports = append(ports, fmt.Sprintf("%d/%s", *port.Port, protocol))
					}
				}
				if len(ports) > 0 {
					metadata["ports"] = strings.Join(ports, ",")
				}
			}
		}

		// Containers Information
		if len(props.Containers) > 0 {
			metadata["containers_count"] = fmt.Sprintf("%d", len(props.Containers))

			var containerNames []string
			var containerImages []string
			for _, container := range props.Containers {
				if container.Name != nil {
					containerNames = append(containerNames, *container.Name)
				}
				if container.Properties != nil && container.Properties.Image != nil {
					containerImages = append(containerImages, *container.Properties.Image)
				}
			}
			if len(containerNames) > 0 {
				metadata["container_names"] = strings.Join(containerNames, ",")
			}
			if len(containerImages) > 0 {
				metadata["container_images"] = strings.Join(containerImages, ",")
			}
		}

		// Image Registry Credentials
		if len(props.ImageRegistryCredentials) > 0 {
			var registries []string
			for _, cred := range props.ImageRegistryCredentials {
				if cred.Server != nil {
					registries = append(registries, *cred.Server)
				}
			}
			if len(registries) > 0 {
				metadata["image_registry_servers"] = strings.Join(registries, ",")
			}
		}

		// Init Containers
		if len(props.InitContainers) > 0 {
			metadata["init_containers_count"] = fmt.Sprintf("%d", len(props.InitContainers))
		}

		// Diagnostics
		if props.Diagnostics != nil && props.Diagnostics.LogAnalytics != nil {
			schema.AddMetadata(metadata, "log_analytics_workspace_id", props.Diagnostics.LogAnalytics.WorkspaceID)
		}

		// Subnet IDs (VNet integration)
		if len(props.SubnetIDs) > 0 {
			var subnetIDs []string
			for _, subnetID := range props.SubnetIDs {
				if subnetID.ID != nil {
					subnetIDs = append(subnetIDs, *subnetID.ID)
				}
			}
			if len(subnetIDs) > 0 {
				metadata["subnet_ids"] = strings.Join(subnetIDs, ",")
			}
		}

		// DNS Config
		if props.DNSConfig != nil {
			if len(props.DNSConfig.NameServers) > 0 {
				var nameServers []string
				for _, ns := range props.DNSConfig.NameServers {
					if ns != nil {
						nameServers = append(nameServers, *ns)
					}
				}
				if len(nameServers) > 0 {
					metadata["dns_name_servers"] = strings.Join(nameServers, ",")
				}
			}
			schema.AddMetadata(metadata, "dns_search_domains", props.DNSConfig.SearchDomains)
		}

		// Encryption Properties
		if props.EncryptionProperties != nil && props.EncryptionProperties.VaultBaseURL != nil {
			metadata["encryption_vault_url"] = *props.EncryptionProperties.VaultBaseURL
		}
	}

	// Zones
	if len(cg.Zones) > 0 {
		var zones []string
		for _, z := range cg.Zones {
			if z != nil {
				zones = append(zones, *z)
			}
		}
		if len(zones) > 0 {
			metadata["availability_zones"] = strings.Join(zones, ",")
		}
	}

	// Tags
	if len(cg.Tags) > 0 {
		if tagString := buildAzureTagString(cg.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// Identity
	if cg.Identity != nil && cg.Identity.Type != nil {
		identityType := string(*cg.Identity.Type)
		metadata["identity_type"] = identityType
		if cg.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *cg.Identity.PrincipalID
		}
	}

	return metadata
}
