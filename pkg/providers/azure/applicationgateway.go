package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// applicationGatewayProvider is a provider for Azure Application Gateway
type applicationGatewayProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (agp *applicationGatewayProvider) name() string {
	return "applicationgateway"
}

// GetResource returns all the Application Gateway resources for a provider.
func (agp *applicationGatewayProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	gateways, err := agp.fetchApplicationGateways(ctx)
	if err != nil {
		return nil, err
	}

	for _, gateway := range gateways {
		// Extract resources from frontend IP configurations
		if gateway.Properties != nil && gateway.Properties.FrontendIPConfigurations != nil {
			for _, frontendIP := range gateway.Properties.FrontendIPConfigurations {
				if frontendIP.Properties == nil {
					continue
				}

				var metadata map[string]string
				if agp.extendedMetadata {
					metadata = agp.getApplicationGatewayMetadata(gateway)
				}

				// Handle public IP (need to resolve the reference)
				if frontendIP.Properties.PublicIPAddress != nil && frontendIP.Properties.PublicIPAddress.ID != nil {
					pipName, pipRG := parseAzureResourceID(*frontendIP.Properties.PublicIPAddress.ID)
					if pipName != "" && pipRG != "" {
						publicIP, err := agp.fetchPublicIPAddress(ctx, pipRG, pipName)
						if err != nil {
							gologger.Warning().Msgf("error fetching public IP %s for application gateway: %s", pipName, err)
							continue
						}

						if publicIP.Properties != nil && publicIP.Properties.IPAddress != nil {
							resource := &schema.Resource{
								Provider: providerName,
								ID:       agp.id,
								Service:  agp.name(),
								Metadata: metadata,
							}

							// Determine IP version
							if publicIP.Properties.PublicIPAddressVersion != nil && *publicIP.Properties.PublicIPAddressVersion == armnetwork.IPVersionIPv4 {
								resource.PublicIPv4 = *publicIP.Properties.IPAddress
							} else if publicIP.Properties.PublicIPAddressVersion != nil {
								resource.PublicIPv6 = *publicIP.Properties.IPAddress
							} else {
								// Default to IPv4 if not specified
								resource.PublicIPv4 = *publicIP.Properties.IPAddress
							}

							list.Append(resource)

							// Add DNS resource if FQDN is available
							if publicIP.Properties.DNSSettings != nil && publicIP.Properties.DNSSettings.Fqdn != nil {
								dnsResource := &schema.Resource{
									Provider: providerName,
									ID:       agp.id,
									DNSName:  *publicIP.Properties.DNSSettings.Fqdn,
									Service:  agp.name(),
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
					}
				}

				// Handle private IP
				if frontendIP.Properties.PrivateIPAddress != nil {
					resource := &schema.Resource{
						Provider:    providerName,
						ID:          agp.id,
						PrivateIpv4: *frontendIP.Properties.PrivateIPAddress,
						Service:     agp.name(),
					}
					if agp.extendedMetadata {
						resource.Metadata = agp.getApplicationGatewayMetadata(gateway)
					}
					list.Append(resource)
				}
			}
		}

		// Handle custom domains configured in HTTP listeners
		if agp.extendedMetadata && gateway.Properties != nil && gateway.Properties.HTTPListeners != nil {
			var metadata map[string]string
			if agp.extendedMetadata {
				metadata = agp.getApplicationGatewayMetadata(gateway)
			}

			for _, listener := range gateway.Properties.HTTPListeners {
				if listener.Properties != nil && listener.Properties.HostNames != nil {
					for _, hostname := range listener.Properties.HostNames {
						if hostname != nil && *hostname != "" {
							dnsResource := &schema.Resource{
								Provider: providerName,
								ID:       agp.id,
								DNSName:  *hostname,
								Service:  agp.name(),
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
				}
			}
		}
	}

	return list, nil
}

func (agp *applicationGatewayProvider) fetchApplicationGateways(ctx context.Context) ([]*armnetwork.ApplicationGateway, error) {
	// Create application gateways client
	client, err := armnetwork.NewApplicationGatewaysClient(agp.SubscriptionID, agp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create application gateways client: %w", err)
	}

	// Use pager pattern to list all application gateways
	var gateways []*armnetwork.ApplicationGateway
	pager := client.NewListAllPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list application gateways: %w", err)
		}

		gateways = append(gateways, page.Value...)
	}

	return gateways, nil
}

func (agp *applicationGatewayProvider) fetchPublicIPAddress(ctx context.Context, resourceGroup, publicIPName string) (*armnetwork.PublicIPAddress, error) {
	// Create public IP addresses client
	ipClient, err := armnetwork.NewPublicIPAddressesClient(agp.SubscriptionID, agp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create public IP client: %w", err)
	}

	resp, err := ipClient.Get(ctx, resourceGroup, publicIPName, nil)
	if err != nil {
		return nil, err
	}

	return &resp.PublicIPAddress, nil
}

func (agp *applicationGatewayProvider) getApplicationGatewayMetadata(gateway *armnetwork.ApplicationGateway) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "gateway_name", gateway.Name)
	schema.AddMetadata(metadata, "gateway_id", gateway.ID)
	metadata["subscription_id"] = agp.SubscriptionID
	metadata["owner_id"] = agp.SubscriptionID
	schema.AddMetadata(metadata, "location", gateway.Location)
	schema.AddMetadata(metadata, "type", gateway.Type)

	// Parse resource group from ID
	if gateway.ID != nil {
		parts := strings.Split(*gateway.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	if gateway.Properties != nil {
		props := gateway.Properties

		// Operational state
		if props.OperationalState != nil {
			operationalState := string(*props.OperationalState)
			metadata["operational_state"] = operationalState
		}

		// Provisioning state
		if props.ProvisioningState != nil {
			provisioningState := string(*props.ProvisioningState)
			schema.AddMetadata(metadata, "provisioning_state", &provisioningState)
		}

		// Backend address pools
		if props.BackendAddressPools != nil {
			metadata["backend_pools_count"] = fmt.Sprintf("%d", len(props.BackendAddressPools))

			var poolNames []string
			for _, pool := range props.BackendAddressPools {
				if pool.Name != nil {
					poolNames = append(poolNames, *pool.Name)
				}
			}
			if len(poolNames) > 0 {
				metadata["backend_address_pools"] = strings.Join(poolNames, ",")
			}
		}

		// Frontend ports
		if props.FrontendPorts != nil {
			var ports []string
			for _, port := range props.FrontendPorts {
				if port.Properties != nil && port.Properties.Port != nil {
					ports = append(ports, fmt.Sprintf("%d", *port.Properties.Port))
				}
			}
			if len(ports) > 0 {
				metadata["frontend_ports"] = strings.Join(ports, ",")
			}
		}

		// Frontend IPs count
		if props.FrontendIPConfigurations != nil {
			metadata["frontend_ips_count"] = fmt.Sprintf("%d", len(props.FrontendIPConfigurations))
		}

		// HTTP listeners
		if props.HTTPListeners != nil {
			metadata["listeners_count"] = fmt.Sprintf("%d", len(props.HTTPListeners))

			var listenerNames []string
			for _, listener := range props.HTTPListeners {
				if listener.Name != nil {
					listenerNames = append(listenerNames, *listener.Name)
				}
			}
			if len(listenerNames) > 0 {
				metadata["listeners"] = strings.Join(listenerNames, ",")
			}
		}

		// Request routing rules
		if props.RequestRoutingRules != nil {
			metadata["routing_rules_count"] = fmt.Sprintf("%d", len(props.RequestRoutingRules))

			var ruleNames []string
			for _, rule := range props.RequestRoutingRules {
				if rule.Name != nil {
					ruleNames = append(ruleNames, *rule.Name)
				}
			}
			if len(ruleNames) > 0 {
				metadata["routing_rules"] = strings.Join(ruleNames, ",")
			}
		}

		// HTTP2 enabled
		if props.EnableHTTP2 != nil {
			metadata["http2_enabled"] = fmt.Sprintf("%v", *props.EnableHTTP2)
		}

		// Web Application Firewall (WAF)
		if props.WebApplicationFirewallConfiguration != nil {
			metadata["waf_enabled"] = "true"
			if props.WebApplicationFirewallConfiguration.Enabled != nil {
				metadata["waf_enabled"] = fmt.Sprintf("%v", *props.WebApplicationFirewallConfiguration.Enabled)
			}
			if props.WebApplicationFirewallConfiguration.FirewallMode != nil {
				wafMode := string(*props.WebApplicationFirewallConfiguration.FirewallMode)
				metadata["waf_mode"] = wafMode
			}
		} else {
			metadata["waf_enabled"] = "false"
		}

		// SSL policy
		if props.SSLPolicy != nil {
			if props.SSLPolicy.PolicyType != nil {
				policyType := string(*props.SSLPolicy.PolicyType)
				schema.AddMetadata(metadata, "ssl_policy_type", &policyType)
			}
			if props.SSLPolicy.PolicyName != nil {
				policyName := string(*props.SSLPolicy.PolicyName)
				schema.AddMetadata(metadata, "ssl_policy_name", &policyName)
			}
		}

		// Redirect configurations count
		if props.RedirectConfigurations != nil {
			metadata["redirect_configs_count"] = fmt.Sprintf("%d", len(props.RedirectConfigurations))
		}

		// URL path maps count
		if props.URLPathMaps != nil {
			metadata["url_path_maps_count"] = fmt.Sprintf("%d", len(props.URLPathMaps))
		}

		// Probes count
		if props.Probes != nil {
			metadata["probes_count"] = fmt.Sprintf("%d", len(props.Probes))
		}

		// Autoscale configuration
		if props.AutoscaleConfiguration != nil {
			if props.AutoscaleConfiguration.MinCapacity != nil {
				metadata["autoscale_min_capacity"] = fmt.Sprintf("%d", *props.AutoscaleConfiguration.MinCapacity)
			}
			if props.AutoscaleConfiguration.MaxCapacity != nil {
				metadata["autoscale_max_capacity"] = fmt.Sprintf("%d", *props.AutoscaleConfiguration.MaxCapacity)
			}
		}

		// SKU information (in Properties)
		if props.SKU != nil {
			if props.SKU.Name != nil {
				skuName := string(*props.SKU.Name)
				schema.AddMetadata(metadata, "sku_name", &skuName)
			}
			if props.SKU.Tier != nil {
				skuTier := string(*props.SKU.Tier)
				schema.AddMetadata(metadata, "sku_tier", &skuTier)
			}
			if props.SKU.Capacity != nil {
				metadata["sku_capacity"] = fmt.Sprintf("%d", *props.SKU.Capacity)
			}
		}
	}

	// Availability zones
	if len(gateway.Zones) > 0 {
		var zones []string
		for _, z := range gateway.Zones {
			if z != nil {
				zones = append(zones, *z)
			}
		}
		if len(zones) > 0 {
			metadata["availability_zones"] = strings.Join(zones, ",")
		}
	}

	// Tags
	if len(gateway.Tags) > 0 {
		if tagString := buildAzureTagString(gateway.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	// Identity
	if gateway.Identity != nil && gateway.Identity.Type != nil {
		identityType := string(*gateway.Identity.Type)
		schema.AddMetadata(metadata, "identity_type", &identityType)
		if gateway.Identity.PrincipalID != nil {
			metadata["identity_principal_id"] = *gateway.Identity.PrincipalID
		}
	}

	return metadata
}
