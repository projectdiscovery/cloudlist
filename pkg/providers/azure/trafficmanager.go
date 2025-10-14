package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/trafficmanager/armtrafficmanager"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// trafficManagerProvider is a provider for Azure Traffic Manager Profiles
type trafficManagerProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential // Track 2: replaced autorest.Authorizer
	extendedMetadata bool
}

// name returns the name of the provider
func (tmp *trafficManagerProvider) name() string {
	return "trafficmanager"
}

// GetResource returns all the Traffic Manager hostnames for a provider.
func (tmp *trafficManagerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	profiles, err := tmp.fetchTrafficManagerProfiles(ctx)
	if err != nil {
		return nil, err
	}

	for _, profile := range profiles {
		if profile.Properties != nil && profile.Properties.DNSConfig != nil && profile.Properties.DNSConfig.Fqdn != nil {
			var metadata map[string]string
			if tmp.extendedMetadata {
				metadata = tmp.getTrafficManagerMetadata(profile)
			}

			resource := &schema.Resource{
				Provider: providerName,
				ID:       tmp.id,
				DNSName:  *profile.Properties.DNSConfig.Fqdn,
				Service:  tmp.name(),
				Metadata: metadata,
			}
			list.Append(resource)
		}
	}
	return list, nil
}

func (tmp *trafficManagerProvider) fetchTrafficManagerProfiles(ctx context.Context) ([]*armtrafficmanager.Profile, error) {
	// Track 2: Create profiles client directly
	client, err := armtrafficmanager.NewProfilesClient(tmp.SubscriptionID, tmp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create traffic manager profiles client: %w", err)
	}

	// Track 2: Use pager pattern
	var profiles []*armtrafficmanager.Profile
	pager := client.NewListBySubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list traffic manager profiles: %w", err)
		}

		profiles = append(profiles, page.Value...)
	}

	return profiles, nil
}

func (tmp *trafficManagerProvider) getTrafficManagerMetadata(profile *armtrafficmanager.Profile) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "profile_name", profile.Name)
	schema.AddMetadata(metadata, "profile_id", profile.ID)
	metadata["subscription_id"] = tmp.SubscriptionID
	metadata["owner_id"] = tmp.SubscriptionID
	schema.AddMetadata(metadata, "location", profile.Location)
	schema.AddMetadata(metadata, "type", profile.Type)

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

	if profile.Properties != nil {
		props := profile.Properties

		if props.ProfileStatus != nil {
			profileStatus := string(*props.ProfileStatus)
			metadata["profile_status"] = profileStatus
		}

		if props.TrafficRoutingMethod != nil {
			routingMethod := string(*props.TrafficRoutingMethod)
			metadata["traffic_routing_method"] = routingMethod
		}

		if props.TrafficViewEnrollmentStatus != nil {
			trafficViewStatus := string(*props.TrafficViewEnrollmentStatus)
			metadata["traffic_view_enrollment_status"] = trafficViewStatus
		}

		if props.MaxReturn != nil {
			metadata["max_return"] = fmt.Sprintf("%d", *props.MaxReturn)
		}

		if props.DNSConfig != nil {
			schema.AddMetadata(metadata, "dns_relative_name", props.DNSConfig.RelativeName)
			schema.AddMetadata(metadata, "dns_fqdn", props.DNSConfig.Fqdn)
			if props.DNSConfig.TTL != nil {
				metadata["dns_ttl"] = fmt.Sprintf("%d", *props.DNSConfig.TTL)
			}
		}

		if props.MonitorConfig != nil {
			if props.MonitorConfig.ProfileMonitorStatus != nil {
				monitorStatus := string(*props.MonitorConfig.ProfileMonitorStatus)
				metadata["monitor_status"] = monitorStatus
			}
			if props.MonitorConfig.Protocol != nil {
				protocol := string(*props.MonitorConfig.Protocol)
				metadata["monitor_protocol"] = protocol
			}
			if props.MonitorConfig.Port != nil {
				metadata["monitor_port"] = fmt.Sprintf("%d", *props.MonitorConfig.Port)
			}
			schema.AddMetadata(metadata, "monitor_path", props.MonitorConfig.Path)
			if props.MonitorConfig.IntervalInSeconds != nil {
				metadata["monitor_interval_seconds"] = fmt.Sprintf("%d", *props.MonitorConfig.IntervalInSeconds)
			}
			if props.MonitorConfig.TimeoutInSeconds != nil {
				metadata["monitor_timeout_seconds"] = fmt.Sprintf("%d", *props.MonitorConfig.TimeoutInSeconds)
			}
			if props.MonitorConfig.ToleratedNumberOfFailures != nil {
				metadata["monitor_tolerated_failures"] = fmt.Sprintf("%d", *props.MonitorConfig.ToleratedNumberOfFailures)
			}

			if props.MonitorConfig.CustomHeaders != nil && len(props.MonitorConfig.CustomHeaders) > 0 {
				var headers []string
				for _, header := range props.MonitorConfig.CustomHeaders {
					if header.Name != nil && header.Value != nil {
						headers = append(headers, fmt.Sprintf("%s=%s", *header.Name, *header.Value))
					}
				}
				if len(headers) > 0 {
					metadata["monitor_custom_headers"] = strings.Join(headers, ",")
				}
			}

			if props.MonitorConfig.ExpectedStatusCodeRanges != nil && len(props.MonitorConfig.ExpectedStatusCodeRanges) > 0 {
				var ranges []string
				for _, statusRange := range props.MonitorConfig.ExpectedStatusCodeRanges {
					if statusRange.Min != nil && statusRange.Max != nil {
						ranges = append(ranges, fmt.Sprintf("%d-%d", *statusRange.Min, *statusRange.Max))
					}
				}
				if len(ranges) > 0 {
					metadata["monitor_expected_status_ranges"] = strings.Join(ranges, ",")
				}
			}
		}

		if props.Endpoints != nil && len(props.Endpoints) > 0 {
			metadata["endpoints_count"] = fmt.Sprintf("%d", len(props.Endpoints))

			var endpointTargets []string
			var endpointTypes []string
			for _, endpoint := range props.Endpoints {
				if endpoint.Properties != nil {
					if endpoint.Properties.Target != nil {
						endpointTargets = append(endpointTargets, *endpoint.Properties.Target)
					}
					if endpoint.Type != nil {
						endpointTypes = append(endpointTypes, *endpoint.Type)
					}
				}
			}
			if len(endpointTargets) > 0 {
				metadata["endpoint_targets"] = strings.Join(endpointTargets, ",")
			}
			if len(endpointTypes) > 0 {
				metadata["endpoint_types"] = strings.Join(endpointTypes, ",")
			}
		}

		if len(props.AllowedEndpointRecordTypes) > 0 {
			var recordTypes []string
			for _, rt := range props.AllowedEndpointRecordTypes {
				if rt != nil {
					recordTypes = append(recordTypes, string(*rt))
				}
			}
			metadata["allowed_endpoint_record_types"] = strings.Join(recordTypes, ",")
		}
	}

	if len(profile.Tags) > 0 {
		if tagString := buildAzureTagString(profile.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}
