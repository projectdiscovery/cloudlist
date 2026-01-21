package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/frontdoor/armfrontdoor"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// frontDoorProvider is a provider for Azure Front Door Profiles
type frontDoorProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential // Track 2: replaced autorest.Authorizer
	extendedMetadata bool
}

// name returns the name of the provider
func (fd *frontDoorProvider) name() string {
	return "frontdoor"
}

// GetResource returns all the Front Door endpoints for a provider.
func (fd *frontDoorProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	frontDoors, err := fd.fetchFrontDoors(ctx)
	if err != nil {
		return nil, err
	}

	for _, frontDoor := range frontDoors {
		if frontDoor.Name == nil || frontDoor.Properties == nil {
			continue
		}

		// Extract frontend endpoints
		if frontDoor.Properties.FrontendEndpoints != nil {
			for _, frontendEndpoint := range frontDoor.Properties.FrontendEndpoints {
				if frontendEndpoint.Properties == nil || frontendEndpoint.Properties.HostName == nil {
					continue
				}

				var metadata map[string]string
				if fd.extendedMetadata {
					metadata = fd.getFrontDoorMetadata(frontDoor, frontendEndpoint)
				}

				// Create resource for frontend endpoint hostname
				resource := &schema.Resource{
					Provider: providerName,
					ID:       fd.id,
					DNSName:  *frontendEndpoint.Properties.HostName,
					Service:  fd.name(),
					Metadata: metadata,
				}
				list.Append(resource)
			}
		}
	}

	return list, nil
}

func (fd *frontDoorProvider) fetchFrontDoors(ctx context.Context) ([]*armfrontdoor.FrontDoor, error) {
	// Track 2: Create Front Doors client directly
	client, err := armfrontdoor.NewFrontDoorsClient(fd.SubscriptionID, fd.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Front Doors client: %w", err)
	}

	// Track 2: Use pager pattern
	var frontDoors []*armfrontdoor.FrontDoor
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list Front Doors: %w", err)
		}

		frontDoors = append(frontDoors, page.Value...)
	}

	return frontDoors, nil
}

func (fd *frontDoorProvider) getFrontDoorMetadata(frontDoor *armfrontdoor.FrontDoor, frontendEndpoint *armfrontdoor.FrontendEndpoint) map[string]string {
	metadata := make(map[string]string)

	// Front Door metadata
	schema.AddMetadata(metadata, "frontdoor_name", frontDoor.Name)
	schema.AddMetadata(metadata, "frontdoor_id", frontDoor.ID)
	metadata["subscription_id"] = fd.SubscriptionID
	metadata["owner_id"] = fd.SubscriptionID
	schema.AddMetadata(metadata, "location", frontDoor.Location)
	schema.AddMetadata(metadata, "type", frontDoor.Type)

	// Track 2: Parse resource group from ID manually
	if frontDoor.ID != nil {
		parts := strings.Split(*frontDoor.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	// Front Door properties
	if frontDoor.Properties != nil {
		props := frontDoor.Properties

		if props.ResourceState != nil {
			resourceState := string(*props.ResourceState)
			metadata["state"] = resourceState
		}

		if props.ProvisioningState != nil {
			metadata["provisioning_state"] = *props.ProvisioningState
		}

		if props.Cname != nil {
			metadata["cname"] = *props.Cname
		}

		if props.FriendlyName != nil {
			metadata["friendly_name"] = *props.FriendlyName
		}

		if props.EnabledState != nil {
			enabledState := string(*props.EnabledState)
			metadata["enabled_state"] = enabledState
		}

		// Frontend endpoints count
		if props.FrontendEndpoints != nil {
			metadata["frontend_endpoints_count"] = fmt.Sprintf("%d", len(props.FrontendEndpoints))

			// List all frontend endpoint hostnames
			var endpointHostnames []string
			for _, fe := range props.FrontendEndpoints {
				if fe.Properties != nil && fe.Properties.HostName != nil {
					endpointHostnames = append(endpointHostnames, *fe.Properties.HostName)
				}
			}
			if len(endpointHostnames) > 0 {
				metadata["frontend_endpoints"] = strings.Join(endpointHostnames, ",")
			}
		}

		// Backend pools count
		if props.BackendPools != nil {
			metadata["backend_pools_count"] = fmt.Sprintf("%d", len(props.BackendPools))

			// List backend pool names
			var poolNames []string
			for _, pool := range props.BackendPools {
				if pool.Name != nil {
					poolNames = append(poolNames, *pool.Name)
				}
			}
			if len(poolNames) > 0 {
				metadata["backend_pools"] = strings.Join(poolNames, ",")
			}
		}

		// Routing rules count
		if props.RoutingRules != nil {
			metadata["routing_rules_count"] = fmt.Sprintf("%d", len(props.RoutingRules))

			// List routing rule names
			var ruleNames []string
			for _, rule := range props.RoutingRules {
				if rule.Name != nil {
					ruleNames = append(ruleNames, *rule.Name)
				}
			}
			if len(ruleNames) > 0 {
				metadata["routing_rules"] = strings.Join(ruleNames, ",")
			}
		}

		// Load balancing settings count
		if props.LoadBalancingSettings != nil {
			metadata["load_balancing_settings_count"] = fmt.Sprintf("%d", len(props.LoadBalancingSettings))
		}

		// Health probe settings count
		if props.HealthProbeSettings != nil {
			metadata["health_probe_settings_count"] = fmt.Sprintf("%d", len(props.HealthProbeSettings))
		}

		// Backend pools advanced info
		if props.BackendPoolsSettings != nil {
			if props.BackendPoolsSettings.EnforceCertificateNameCheck != nil {
				enforceCertCheck := string(*props.BackendPoolsSettings.EnforceCertificateNameCheck)
				metadata["enforce_certificate_name_check"] = enforceCertCheck
			}
			if props.BackendPoolsSettings.SendRecvTimeoutSeconds != nil {
				metadata["send_recv_timeout_seconds"] = fmt.Sprintf("%d", *props.BackendPoolsSettings.SendRecvTimeoutSeconds)
			}
		}
	}

	// Frontend endpoint specific metadata
	schema.AddMetadata(metadata, "frontend_endpoint_name", frontendEndpoint.Name)
	schema.AddMetadata(metadata, "frontend_endpoint_id", frontendEndpoint.ID)

	if frontendEndpoint.Properties != nil {
		if frontendEndpoint.Properties.SessionAffinityEnabledState != nil {
			sessionAffinityState := string(*frontendEndpoint.Properties.SessionAffinityEnabledState)
			metadata["session_affinity_enabled"] = sessionAffinityState
		}

		if frontendEndpoint.Properties.SessionAffinityTTLSeconds != nil {
			metadata["session_affinity_ttl_seconds"] = fmt.Sprintf("%d", *frontendEndpoint.Properties.SessionAffinityTTLSeconds)
		}

		if frontendEndpoint.Properties.WebApplicationFirewallPolicyLink != nil {
			schema.AddMetadata(metadata, "waf_policy_id", frontendEndpoint.Properties.WebApplicationFirewallPolicyLink.ID)
		}

		if frontendEndpoint.Properties.ResourceState != nil {
			feResourceState := string(*frontendEndpoint.Properties.ResourceState)
			metadata["frontend_endpoint_state"] = feResourceState
		}

		if frontendEndpoint.Properties.CustomHTTPSProvisioningState != nil {
			httpsProvisioningState := string(*frontendEndpoint.Properties.CustomHTTPSProvisioningState)
			metadata["custom_https_provisioning_state"] = httpsProvisioningState
		}

		if frontendEndpoint.Properties.CustomHTTPSConfiguration != nil {
			httpsConfig := frontendEndpoint.Properties.CustomHTTPSConfiguration
			if httpsConfig.CertificateSource != nil {
				certSource := string(*httpsConfig.CertificateSource)
				metadata["custom_https_certificate_source"] = certSource
			}
			if httpsConfig.ProtocolType != nil {
				protocolType := string(*httpsConfig.ProtocolType)
				metadata["custom_https_protocol_type"] = protocolType
			}
			if httpsConfig.MinimumTLSVersion != nil {
				minTLSVersion := string(*httpsConfig.MinimumTLSVersion)
				metadata["minimum_tls_version"] = minTLSVersion
			}
		}
	}

	// Tags
	if len(frontDoor.Tags) > 0 {
		if tagString := buildAzureTagString(frontDoor.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}
