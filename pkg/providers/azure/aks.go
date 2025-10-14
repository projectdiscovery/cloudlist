package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// aksProvider is a provider for Azure Kubernetes Service (AKS)
type aksProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (ap *aksProvider) name() string {
	return "aks"
}

// GetResource returns all the AKS cluster resources
func (ap *aksProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	clusters, err := ap.fetchAKSClusters(ctx)
	if err != nil {
		return nil, err
	}

	for _, cluster := range clusters {
		if cluster.Properties == nil {
			continue
		}

		// Extract cluster FQDN as primary DNS resource
		if cluster.Properties.Fqdn != nil && *cluster.Properties.Fqdn != "" {
			var metadata map[string]string
			if ap.extendedMetadata {
				metadata = ap.getAKSMetadata(cluster)
			}

			resource := &schema.Resource{
				Provider: providerName,
				ID:       ap.id,
				DNSName:  *cluster.Properties.Fqdn,
				Service:  ap.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Extract private FQDN if available
		if cluster.Properties.PrivateFQDN != nil && *cluster.Properties.PrivateFQDN != "" {
			var metadata map[string]string
			if ap.extendedMetadata {
				metadata = ap.getAKSMetadata(cluster)
			}

			resource := &schema.Resource{
				Provider: providerName,
				ID:       ap.id,
				DNSName:  *cluster.Properties.PrivateFQDN,
				Service:  ap.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}

		// Extract LoadBalancer IPs from agent pools
		if cluster.Properties.AgentPoolProfiles != nil {
			for _, pool := range cluster.Properties.AgentPoolProfiles {
				if pool.Name == nil {
					continue
				}

				// Get public IPs from node resource group if available
				// Note: LoadBalancer IPs are in the managed node resource group
				// This requires additional API calls to the network resource provider
				// We'll extract what's available from the cluster properties directly

				// Extract node pool metadata if extended metadata is enabled
				if ap.extendedMetadata {
					// Node pools don't have direct IP exposure in the cluster object
					// IPs are managed by Azure in the node resource group
					continue
				}
			}
		}

		// Extract network profile information for potential IPs
		if cluster.Properties.NetworkProfile != nil {
			networkProfile := cluster.Properties.NetworkProfile

			// LoadBalancer outbound IPs are in the managed infrastructure
			// These are not directly exposed in the cluster API response
			// To get LoadBalancer service IPs, we would need to:
			// 1. Connect to the Kubernetes API using the cluster credentials
			// 2. List services with type=LoadBalancer
			// 3. Extract their external IPs
			// This is beyond the scope of Azure Resource Manager API

			// However, we can extract service CIDR information for metadata
			if ap.extendedMetadata && networkProfile.ServiceCidr != nil {
				// Service CIDR is internal network range, not public IPs
				// Adding to metadata only
			}
		}
	}

	return list, nil
}

func (ap *aksProvider) fetchAKSClusters(ctx context.Context) ([]*armcontainerservice.ManagedCluster, error) {
	// Track 2: Create managed clusters client directly
	client, err := armcontainerservice.NewManagedClustersClient(ap.SubscriptionID, ap.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create AKS managed clusters client: %w", err)
	}

	// Track 2: Use pager pattern
	var clusters []*armcontainerservice.ManagedCluster
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list AKS clusters: %w", err)
		}

		clusters = append(clusters, page.Value...)
	}

	return clusters, nil
}

func (ap *aksProvider) getAKSMetadata(cluster *armcontainerservice.ManagedCluster) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "cluster_name", cluster.Name)
	schema.AddMetadata(metadata, "cluster_id", cluster.ID)
	metadata["subscription_id"] = ap.SubscriptionID
	metadata["owner_id"] = ap.SubscriptionID
	schema.AddMetadata(metadata, "location", cluster.Location)
	schema.AddMetadata(metadata, "type", cluster.Type)

	// Parse resource group from ID
	if cluster.ID != nil {
		_, resourceGroup := parseAzureResourceID(*cluster.ID)
		if resourceGroup != "" {
			metadata["resource_group"] = resourceGroup
		}
	}

	if cluster.Properties != nil {
		props := cluster.Properties

		// Kubernetes version
		schema.AddMetadata(metadata, "kubernetes_version", props.KubernetesVersion)

		// DNS prefix and FQDN
		schema.AddMetadata(metadata, "dns_prefix", props.DNSPrefix)
		schema.AddMetadata(metadata, "fqdn", props.Fqdn)
		schema.AddMetadata(metadata, "private_fqdn", props.PrivateFQDN)

		// Node resource group (managed by Azure)
		schema.AddMetadata(metadata, "node_resource_group", props.NodeResourceGroup)

		// Provisioning state
		schema.AddMetadata(metadata, "provisioning_state", props.ProvisioningState)

		// Power state
		if props.PowerState != nil && props.PowerState.Code != nil {
			powerState := string(*props.PowerState.Code)
			metadata["power_state"] = powerState
		}

		// SKU tier
		if cluster.SKU != nil && cluster.SKU.Tier != nil {
			skuTier := string(*cluster.SKU.Tier)
			metadata["sku_tier"] = skuTier
		}

		// Network profile
		if props.NetworkProfile != nil {
			networkProfile := props.NetworkProfile

			if networkProfile.NetworkPlugin != nil {
				networkPlugin := string(*networkProfile.NetworkPlugin)
				metadata["network_plugin"] = networkPlugin
			}

			if networkProfile.NetworkPolicy != nil {
				networkPolicy := string(*networkProfile.NetworkPolicy)
				metadata["network_policy"] = networkPolicy
			}

			if networkProfile.NetworkMode != nil {
				networkMode := string(*networkProfile.NetworkMode)
				metadata["network_mode"] = networkMode
			}

			schema.AddMetadata(metadata, "service_cidr", networkProfile.ServiceCidr)
			schema.AddMetadata(metadata, "dns_service_ip", networkProfile.DNSServiceIP)
			schema.AddMetadata(metadata, "pod_cidr", networkProfile.PodCidr)

			if networkProfile.LoadBalancerProfile != nil {
				lbProfile := networkProfile.LoadBalancerProfile

				if lbProfile.ManagedOutboundIPs != nil && lbProfile.ManagedOutboundIPs.Count != nil {
					metadata["lb_managed_outbound_ips_count"] = fmt.Sprintf("%d", *lbProfile.ManagedOutboundIPs.Count)
				}

				if lbProfile.AllocatedOutboundPorts != nil {
					metadata["lb_allocated_outbound_ports"] = fmt.Sprintf("%d", *lbProfile.AllocatedOutboundPorts)
				}

				if lbProfile.IdleTimeoutInMinutes != nil {
					metadata["lb_idle_timeout_minutes"] = fmt.Sprintf("%d", *lbProfile.IdleTimeoutInMinutes)
				}
			}

			if networkProfile.OutboundType != nil {
				outboundType := string(*networkProfile.OutboundType)
				metadata["outbound_type"] = outboundType
			}
		}

		// API server access profile
		if props.APIServerAccessProfile != nil {
			if props.APIServerAccessProfile.EnablePrivateCluster != nil {
				metadata["private_cluster"] = fmt.Sprintf("%v", *props.APIServerAccessProfile.EnablePrivateCluster)
			}

			if len(props.APIServerAccessProfile.AuthorizedIPRanges) > 0 {
				var ipRanges []string
				for _, ipRange := range props.APIServerAccessProfile.AuthorizedIPRanges {
					if ipRange != nil {
						ipRanges = append(ipRanges, *ipRange)
					}
				}
				if len(ipRanges) > 0 {
					metadata["authorized_ip_ranges"] = strings.Join(ipRanges, ",")
				}
			}
		}

		// Agent pool profiles (node pools)
		if len(props.AgentPoolProfiles) > 0 {
			metadata["agent_pool_count"] = fmt.Sprintf("%d", len(props.AgentPoolProfiles))

			var poolNames []string
			var poolVMSizes []string
			var poolCounts []string

			for _, pool := range props.AgentPoolProfiles {
				if pool.Name != nil {
					poolNames = append(poolNames, *pool.Name)
				}
				if pool.VMSize != nil {
					poolVMSizes = append(poolVMSizes, *pool.VMSize)
				}
				if pool.Count != nil {
					poolCounts = append(poolCounts, fmt.Sprintf("%d", *pool.Count))
				}
			}

			if len(poolNames) > 0 {
				metadata["agent_pool_names"] = strings.Join(poolNames, ",")
			}
			if len(poolVMSizes) > 0 {
				metadata["agent_pool_vm_sizes"] = strings.Join(poolVMSizes, ",")
			}
			if len(poolCounts) > 0 {
				metadata["agent_pool_node_counts"] = strings.Join(poolCounts, ",")
			}
		}

		// Add-ons and features
		if len(props.AddonProfiles) > 0 {
			var enabledAddons []string
			for addonName, addonProfile := range props.AddonProfiles {
				if addonProfile.Enabled != nil && *addonProfile.Enabled {
					enabledAddons = append(enabledAddons, addonName)
				}
			}
			if len(enabledAddons) > 0 {
				metadata["enabled_addons"] = strings.Join(enabledAddons, ",")
			}
		}

		// Azure RBAC enabled
		if props.EnableRBAC != nil {
			metadata["rbac_enabled"] = fmt.Sprintf("%v", *props.EnableRBAC)
		}

		// AAD profile
		if props.AADProfile != nil {
			if props.AADProfile.Managed != nil {
				metadata["aad_managed"] = fmt.Sprintf("%v", *props.AADProfile.Managed)
			}
			if props.AADProfile.EnableAzureRBAC != nil {
				metadata["aad_azure_rbac_enabled"] = fmt.Sprintf("%v", *props.AADProfile.EnableAzureRBAC)
			}
		}

		// Auto-scaler profile
		if props.AutoScalerProfile != nil {
			if props.AutoScalerProfile.ScaleDownDelayAfterAdd != nil {
				metadata["autoscaler_scale_down_delay"] = *props.AutoScalerProfile.ScaleDownDelayAfterAdd
			}
		}

		// Disk encryption
		if props.DiskEncryptionSetID != nil {
			metadata["disk_encryption_set_id"] = *props.DiskEncryptionSetID
		}

		// Auto-upgrade channel
		if props.AutoUpgradeProfile != nil && props.AutoUpgradeProfile.UpgradeChannel != nil {
			upgradeChannel := string(*props.AutoUpgradeProfile.UpgradeChannel)
			metadata["auto_upgrade_channel"] = upgradeChannel
		}

		// HTTP application routing (deprecated but still in API)
		if props.HTTPProxyConfig != nil {
			if props.HTTPProxyConfig.HTTPProxy != nil {
				metadata["http_proxy"] = *props.HTTPProxyConfig.HTTPProxy
			}
			if props.HTTPProxyConfig.HTTPSProxy != nil {
				metadata["https_proxy"] = *props.HTTPProxyConfig.HTTPSProxy
			}
		}

		// Security profile - Note: Some newer security features may not be available in all SDK versions
		// The SecurityProfile field exists but may not have all sub-fields like Defender, AzureKeyVaultKms, WorkloadIdentity
	}

	// Identity
	if cluster.Identity != nil && cluster.Identity.Type != nil {
		identityType := string(*cluster.Identity.Type)
		metadata["identity_type"] = identityType

		if cluster.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *cluster.Identity.PrincipalID
		}

		if cluster.Identity.TenantID != nil {
			metadata["identity_tenant_id"] = *cluster.Identity.TenantID
		}
	}

	// Tags
	if len(cluster.Tags) > 0 {
		if tagString := buildAzureTagString(cluster.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// Extended location (for edge scenarios)
	if cluster.ExtendedLocation != nil {
		schema.AddMetadata(metadata, "extended_location_name", cluster.ExtendedLocation.Name)
		if cluster.ExtendedLocation.Type != nil {
			extType := string(*cluster.ExtendedLocation.Type)
			schema.AddMetadata(metadata, "extended_location_type", &extType)
		}
	}

	return metadata
}
