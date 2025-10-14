package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// dnsProvider is a provider for Azure DNS zones and records
type dnsProvider struct {
	id               string
	SubscriptionID   string
	Credential       azcore.TokenCredential
	extendedMetadata bool
}

// name returns the name of the provider
func (dp *dnsProvider) name() string {
	return "dns"
}

// GetResource returns all DNS records from Azure DNS zones
func (dp *dnsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	zones, err := dp.fetchDNSZones(ctx)
	if err != nil {
		return nil, err
	}

	for _, zone := range zones {
		if zone.Name == nil {
			continue
		}

		// Parse resource group from zone ID
		_, resourceGroup := parseAzureResourceID(*zone.ID)
		if resourceGroup == "" {
			gologger.Warning().Msgf("Failed to parse resource group from zone ID: %s", *zone.ID)
			continue
		}

		// Fetch all record sets in this zone
		recordSets, err := dp.fetchRecordSets(ctx, resourceGroup, *zone.Name)
		if err != nil {
			gologger.Warning().Msgf("Failed to list record sets for zone %s: %s", *zone.Name, err)
			continue
		}

		// Process each record set
		for _, recordSet := range recordSets {
			if recordSet.Name == nil || recordSet.Type == nil {
				continue
			}

			// Extract record type from full type string (e.g., "Microsoft.Network/dnszones/A" -> "A")
			recordType := extractRecordType(*recordSet.Type)

			// Process based on record type
			switch recordType {
			case "A":
				// Process A records (IPv4)
				if recordSet.Properties != nil && recordSet.Properties.ARecords != nil {
					for _, aRecord := range recordSet.Properties.ARecords {
						if aRecord.IPv4Address == nil {
							continue
						}

						var metadata map[string]string
						if dp.extendedMetadata {
							metadata = dp.getDNSRecordMetadata(zone, recordSet, recordType)
						}

						// Build FQDN: <record-name>.<zone-name>
						fqdn := buildFQDN(*recordSet.Name, *zone.Name)

						resource := &schema.Resource{
							Provider:   providerName,
							ID:         dp.id,
							PublicIPv4: *aRecord.IPv4Address,
							DNSName:    fqdn,
							Service:    dp.name(),
							Metadata:   metadata,
						}
						list.Append(resource)
					}
				}

			case "AAAA":
				// Process AAAA records (IPv6)
				if recordSet.Properties != nil && recordSet.Properties.AaaaRecords != nil {
					for _, aaaaRecord := range recordSet.Properties.AaaaRecords {
						if aaaaRecord.IPv6Address == nil {
							continue
						}

						var metadata map[string]string
						if dp.extendedMetadata {
							metadata = dp.getDNSRecordMetadata(zone, recordSet, recordType)
						}

						fqdn := buildFQDN(*recordSet.Name, *zone.Name)

						resource := &schema.Resource{
							Provider:   providerName,
							ID:         dp.id,
							PublicIPv6: *aaaaRecord.IPv6Address,
							DNSName:    fqdn,
							Service:    dp.name(),
							Metadata:   metadata,
						}
						list.Append(resource)
					}
				}

			case "CNAME":
				// Process CNAME records (DNS-only resources)
				if recordSet.Properties != nil && recordSet.Properties.CnameRecord != nil && recordSet.Properties.CnameRecord.Cname != nil {
					var metadata map[string]string
					if dp.extendedMetadata {
						metadata = dp.getDNSRecordMetadata(zone, recordSet, recordType)
						metadata["cname_target"] = *recordSet.Properties.CnameRecord.Cname
					}

					fqdn := buildFQDN(*recordSet.Name, *zone.Name)

					resource := &schema.Resource{
						Provider: providerName,
						ID:       dp.id,
						DNSName:  fqdn,
						Service:  dp.name(),
						Metadata: metadata,
					}
					list.Append(resource)
				}
			}
		}
	}

	return list, nil
}

// fetchDNSZones retrieves all DNS zones in the subscription
func (dp *dnsProvider) fetchDNSZones(ctx context.Context) ([]*armdns.Zone, error) {
	client, err := armdns.NewZonesClient(dp.SubscriptionID, dp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS zones client: %w", err)
	}

	var zones []*armdns.Zone
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list DNS zones: %w", err)
		}

		zones = append(zones, page.Value...)
	}

	return zones, nil
}

// fetchRecordSets retrieves all record sets for a given DNS zone
func (dp *dnsProvider) fetchRecordSets(ctx context.Context, resourceGroup, zoneName string) ([]*armdns.RecordSet, error) {
	client, err := armdns.NewRecordSetsClient(dp.SubscriptionID, dp.Credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS record sets client: %w", err)
	}

	var recordSets []*armdns.RecordSet
	pager := client.NewListByDNSZonePager(resourceGroup, zoneName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list record sets: %w", err)
		}

		recordSets = append(recordSets, page.Value...)
	}

	return recordSets, nil
}

// getDNSRecordMetadata builds extended metadata for a DNS record
func (dp *dnsProvider) getDNSRecordMetadata(zone *armdns.Zone, recordSet *armdns.RecordSet, recordType string) map[string]string {
	metadata := make(map[string]string)

	// Zone information
	schema.AddMetadata(metadata, "zone_name", zone.Name)
	schema.AddMetadata(metadata, "zone_id", zone.ID)
	metadata["subscription_id"] = dp.SubscriptionID
	metadata["owner_id"] = dp.SubscriptionID

	// Parse resource group from zone ID
	if zone.ID != nil {
		parts := strings.Split(*zone.ID, "/")
		for i, part := range parts {
			if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
				metadata["resource_group"] = parts[i+1]
				break
			}
		}
	}

	schema.AddMetadata(metadata, "zone_location", zone.Location)

	// Zone properties
	if zone.Properties != nil {
		if zone.Properties.NumberOfRecordSets != nil {
			metadata["zone_record_count"] = fmt.Sprintf("%d", *zone.Properties.NumberOfRecordSets)
		}
		if zone.Properties.MaxNumberOfRecordSets != nil {
			metadata["zone_max_record_count"] = fmt.Sprintf("%d", *zone.Properties.MaxNumberOfRecordSets)
		}
		if len(zone.Properties.NameServers) > 0 {
			var nameServers []string
			for _, ns := range zone.Properties.NameServers {
				if ns != nil {
					nameServers = append(nameServers, *ns)
				}
			}
			if len(nameServers) > 0 {
				metadata["zone_name_servers"] = strings.Join(nameServers, ",")
			}
		}
		if zone.Properties.ZoneType != nil {
			zoneType := string(*zone.Properties.ZoneType)
			metadata["zone_type"] = zoneType
		}
	}

	// Record set information
	schema.AddMetadata(metadata, "record_name", recordSet.Name)
	schema.AddMetadata(metadata, "record_id", recordSet.ID)
	metadata["record_type"] = recordType

	if recordSet.Properties != nil {
		if recordSet.Properties.TTL != nil {
			metadata["ttl"] = fmt.Sprintf("%d", *recordSet.Properties.TTL)
		}

		schema.AddMetadata(metadata, "record_fqdn", recordSet.Properties.Fqdn)
		schema.AddMetadata(metadata, "provisioning_state", recordSet.Properties.ProvisioningState)

		// Target resource ID for alias records
		if recordSet.Properties.TargetResource != nil {
			schema.AddMetadata(metadata, "target_resource_id", recordSet.Properties.TargetResource.ID)
		}

		// Record-specific metadata
		if len(recordSet.Properties.Metadata) > 0 {
			var metaPairs []string
			for key, value := range recordSet.Properties.Metadata {
				if value != nil {
					metaPairs = append(metaPairs, fmt.Sprintf("%s=%s", key, *value))
				}
			}
			if len(metaPairs) > 0 {
				metadata["record_metadata"] = strings.Join(metaPairs, ",")
			}
		}
	}

	// Zone tags
	if len(zone.Tags) > 0 {
		if tagString := buildAzureTagString(zone.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}

// buildFQDN constructs the fully qualified domain name from record name and zone name
func buildFQDN(recordName, zoneName string) string {
	// Handle @ record (zone apex)
	if recordName == "@" {
		return zoneName
	}
	return fmt.Sprintf("%s.%s", recordName, zoneName)
}

// extractRecordType extracts the record type from the full Azure type string
// Example: "Microsoft.Network/dnszones/A" -> "A"
func extractRecordType(fullType string) string {
	parts := strings.Split(fullType, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
