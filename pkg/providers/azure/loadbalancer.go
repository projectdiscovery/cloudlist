package azure

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/alitto/pond/v2"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// loadBalancerProvider is a provider for Azure Load Balancers
type loadBalancerProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

func (lbp *loadBalancerProvider) name() string {
	return "loadbalancer"
}

// GetResource returns all the load balancer resources for a provider.
func (lbp *loadBalancerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	mu := &sync.Mutex{}

	groups, err := fetchResouceGroups(ctx, lbp.SubscriptionID, lbp.Credential)
	if err != nil {
		return nil, err
	}

	// Create a goroutine pool with size that matches Azure API limitations
	pool := pond.NewPool(10)

	for _, group := range groups {
		group := group // Create local copy for goroutine

		// Submit task to the pool
		pool.Submit(func() {
			resourcesSlice, err := lbp.processResourceGroup(ctx, group)
			if err != nil {
				gologger.Warning().Msgf("error processing resource group %s: %s", group, err)
			}

			mu.Lock()
			for _, resource := range resourcesSlice {
				list.Append(resource)
			}
			mu.Unlock()
		})
	}
	pool.StopAndWait()

	return list, nil
}

func (lbp *loadBalancerProvider) processResourceGroup(ctx context.Context, group string) ([]*schema.Resource, error) {
	lbList, err := lbp.fetchLoadBalancers(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("error fetching load balancer list: %w", err)
	}

	var resources []*schema.Resource

	for _, lb := range lbList {
		// Skip if no properties
		if lb.Properties == nil {
			continue
		}

		// Process frontend IP configurations
		if lb.Properties.FrontendIPConfigurations != nil {
			for _, frontendIPConfig := range lb.Properties.FrontendIPConfigurations {
				if frontendIPConfig.Properties == nil {
					continue
				}

				resource := &schema.Resource{
					Provider: providerName,
					ID:       lbp.id,
					Service:  lbp.name(),
				}

				// Extract private IP (available for both public and internal LBs)
				if frontendIPConfig.Properties.PrivateIPAddress != nil {
					resource.PrivateIpv4 = *frontendIPConfig.Properties.PrivateIPAddress
				}

				// Extract public IP if referenced (only for public LBs)
				if frontendIPConfig.Properties.PublicIPAddress != nil && frontendIPConfig.Properties.PublicIPAddress.ID != nil {
					publicIPName, publicIPRG := parseAzureResourceID(*frontendIPConfig.Properties.PublicIPAddress.ID)
					if publicIPName != "" && publicIPRG != "" {
						publicIP, err := lbp.fetchPublicIP(ctx, publicIPRG, publicIPName)
						if err != nil {
							gologger.Warning().Msgf("error fetching public IP %s: %s", publicIPName, err)
						} else if publicIP.Properties != nil && publicIP.Properties.IPAddress != nil {
							// Set public IP based on version
							if publicIP.Properties.PublicIPAddressVersion != nil && *publicIP.Properties.PublicIPAddressVersion == armnetwork.IPVersionIPv4 {
								resource.PublicIPv4 = *publicIP.Properties.IPAddress
							} else if publicIP.Properties.PublicIPAddressVersion != nil {
								resource.PublicIPv6 = *publicIP.Properties.IPAddress
							} else {
								// Default to IPv4 if not specified
								resource.PublicIPv4 = *publicIP.Properties.IPAddress
							}
							resource.Public = true

							// Add DNS resource if available
							if publicIP.Properties.DNSSettings != nil && publicIP.Properties.DNSSettings.Fqdn != nil {
								dnsResource := &schema.Resource{
									Provider: providerName,
									ID:       lbp.id,
									DNSName:  *publicIP.Properties.DNSSettings.Fqdn,
									Service:  lbp.name(),
								}
								if lbp.extendedMetadata {
									metadata := lbp.getLoadBalancerMetadata(lb, group, frontendIPConfig)
									dnsResource.Metadata = make(map[string]string)
									for k, v := range metadata {
										dnsResource.Metadata[k] = v
									}
								}
								resources = append(resources, dnsResource)
							}
						}
					}
				}

				// Add extended metadata
				if lbp.extendedMetadata {
					resource.Metadata = lbp.getLoadBalancerMetadata(lb, group, frontendIPConfig)
				}

				// Only add resource if it has an IP or DNS name
				if resource.PublicIPv4 != "" || resource.PublicIPv6 != "" || resource.PrivateIpv4 != "" || resource.DNSName != "" {
					resources = append(resources, resource)
				}
			}
		}
	}

	return resources, nil
}

func (lbp *loadBalancerProvider) fetchLoadBalancers(ctx context.Context, group string) ([]*armnetwork.LoadBalancer, error) {
	// Create load balancers client
	lbClient, err := armnetwork.NewLoadBalancersClient(lbp.SubscriptionID, lbp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancers client: %w", err)
	}

	// Use pager pattern
	var lbList []*armnetwork.LoadBalancer
	pager := lbClient.NewListPager(group, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list load balancers: %w", err)
		}

		lbList = append(lbList, page.Value...)
	}

	return lbList, nil
}

func (lbp *loadBalancerProvider) fetchPublicIP(ctx context.Context, group, publicIPName string) (armnetwork.PublicIPAddress, error) {
	// Create public IP addresses client
	ipClient, err := armnetwork.NewPublicIPAddressesClient(lbp.SubscriptionID, lbp.Credential, nil)
	if err != nil {
		return armnetwork.PublicIPAddress{}, fmt.Errorf("failed to create public IP client: %w", err)
	}

	resp, err := ipClient.Get(ctx, group, publicIPName, nil)
	if err != nil {
		return armnetwork.PublicIPAddress{}, err
	}

	return resp.PublicIPAddress, nil
}

func (lbp *loadBalancerProvider) getLoadBalancerMetadata(lb *armnetwork.LoadBalancer, resourceGroup string, frontendIPConfig *armnetwork.FrontendIPConfiguration) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "lb_name", lb.Name)
	schema.AddMetadata(metadata, "lb_id", lb.ID)
	metadata["resource_group"] = resourceGroup
	metadata["subscription_id"] = lbp.SubscriptionID
	metadata["owner_id"] = lbp.SubscriptionID
	schema.AddMetadata(metadata, "location", lb.Location)

	// SKU information
	if lb.SKU != nil {
		if lb.SKU.Name != nil {
			skuName := string(*lb.SKU.Name)
			schema.AddMetadata(metadata, "sku_name", &skuName)
		}
		if lb.SKU.Tier != nil {
			skuTier := string(*lb.SKU.Tier)
			schema.AddMetadata(metadata, "sku_tier", &skuTier)
		}
	}

	if lb.Properties != nil {
		props := lb.Properties

		if props.ProvisioningState != nil {
			provisioningState := string(*props.ProvisioningState)
			schema.AddMetadata(metadata, "provisioning_state", &provisioningState)
		}

		// Frontend IP configurations count
		if props.FrontendIPConfigurations != nil {
			metadata["frontend_ip_count"] = fmt.Sprintf("%d", len(props.FrontendIPConfigurations))

			// List all frontend IPs
			var frontendIPs []string
			for _, fip := range props.FrontendIPConfigurations {
				if fip.Name != nil {
					frontendIPs = append(frontendIPs, *fip.Name)
				}
			}
			if len(frontendIPs) > 0 {
				metadata["frontend_ips"] = strings.Join(frontendIPs, ",")
			}
		}

		// Backend pools count
		if props.BackendAddressPools != nil {
			metadata["backend_pool_count"] = fmt.Sprintf("%d", len(props.BackendAddressPools))

			// List backend pool names
			var backendPools []string
			for _, pool := range props.BackendAddressPools {
				if pool.Name != nil {
					backendPools = append(backendPools, *pool.Name)
				}
			}
			if len(backendPools) > 0 {
				metadata["backend_pools"] = strings.Join(backendPools, ",")
			}
		}

		// Load balancing rules count
		if props.LoadBalancingRules != nil {
			metadata["rules_count"] = fmt.Sprintf("%d", len(props.LoadBalancingRules))

			// List load balancing rules
			var rules []string
			for _, rule := range props.LoadBalancingRules {
				if rule.Name != nil {
					rules = append(rules, *rule.Name)
				}
			}
			if len(rules) > 0 {
				metadata["load_balancing_rules"] = strings.Join(rules, ",")
			}
		}

		// Probes count
		if props.Probes != nil {
			metadata["probes_count"] = fmt.Sprintf("%d", len(props.Probes))

			// List probes
			var probes []string
			for _, probe := range props.Probes {
				if probe.Name != nil {
					probes = append(probes, *probe.Name)
				}
			}
			if len(probes) > 0 {
				metadata["probes"] = strings.Join(probes, ",")
			}
		}

		// Inbound NAT rules
		if props.InboundNatRules != nil && len(props.InboundNatRules) > 0 {
			metadata["inbound_nat_rules_count"] = fmt.Sprintf("%d", len(props.InboundNatRules))
		}

		// Outbound rules
		if props.OutboundRules != nil && len(props.OutboundRules) > 0 {
			metadata["outbound_rules_count"] = fmt.Sprintf("%d", len(props.OutboundRules))
		}
	}

	// Frontend IP configuration specific metadata
	if frontendIPConfig != nil {
		schema.AddMetadata(metadata, "frontend_config_name", frontendIPConfig.Name)
		schema.AddMetadata(metadata, "frontend_config_id", frontendIPConfig.ID)

		if frontendIPConfig.Properties != nil {
			if frontendIPConfig.Properties.PrivateIPAllocationMethod != nil {
				allocationMethod := string(*frontendIPConfig.Properties.PrivateIPAllocationMethod)
				metadata["private_ip_allocation_method"] = allocationMethod
			}

			if frontendIPConfig.Properties.PrivateIPAddressVersion != nil {
				ipVersion := string(*frontendIPConfig.Properties.PrivateIPAddressVersion)
				metadata["private_ip_version"] = ipVersion
			}

			if frontendIPConfig.Properties.Subnet != nil {
				schema.AddMetadata(metadata, "subnet_id", frontendIPConfig.Properties.Subnet.ID)
			}
		}

		// Zones
		if frontendIPConfig.Zones != nil && len(frontendIPConfig.Zones) > 0 {
			var zones []string
			for _, z := range frontendIPConfig.Zones {
				if z != nil {
					zones = append(zones, *z)
				}
			}
			if len(zones) > 0 {
				metadata["availability_zones"] = strings.Join(zones, ",")
			}
		}
	}

	// Tags
	if len(lb.Tags) > 0 {
		if tagString := buildAzureTagString(lb.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}
