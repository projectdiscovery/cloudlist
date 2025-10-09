package gcp

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/dns/v1"
)

// cloudDNSProvider is a provider for aws Route53 API
type cloudDNSProvider struct {
	id               string
	dns              *dns.Service
	projects         []string
	extendedMetadata bool
}

func (d *cloudDNSProvider) name() string {
	return "dns"
}

// GetResource returns all the resources in the store for a provider.
func (d *cloudDNSProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		zone := d.dns.ManagedZones.List(project)
		err := zone.Pages(context.Background(), func(resp *dns.ManagedZonesListResponse) error {
			for _, z := range resp.ManagedZones {
				resources := d.dns.ResourceRecordSets.List(project, z.Name)
				err := resources.Pages(context.Background(), func(r *dns.ResourceRecordSetsListResponse) error {
					items := d.parseRecordsForResourceSet(r, z, project)
					list.Merge(items)
					return nil
				})
				if err != nil {
					log.Printf("Could not get resource_records for zone %s in project %s: %s\n", z.Name, project, err)
					continue
				}
			}
			return nil
		})
		if err != nil {
			log.Printf("Could not get all zones for project %s: %s\n", project, err)
			continue
		}
	}
	return list, nil
}

// parseRecordsForResourceSet parses and returns the records for a resource set
func (d *cloudDNSProvider) parseRecordsForResourceSet(r *dns.ResourceRecordSetsListResponse, zone *dns.ManagedZone, project string) *schema.Resources {
	list := schema.NewResources()

	for _, resource := range r.Rrsets {
		if resource.Type != "A" && resource.Type != "CNAME" && resource.Type != "AAAA" {
			continue
		}

		for _, data := range resource.Rrdatas {
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getRecordMetadata(resource, zone, project, data)
			}

			dst := &schema.Resource{
				DNSName:  resource.Name,
				Public:   true,
				ID:       d.id,
				Provider: providerName,
				Service:  d.name(),
				Metadata: metadata,
			}

			//nolint
			if resource.Type == "A" {
				dst.PublicIPv4 = data
			} else if resource.Type == "AAAA" {
				dst.PublicIPv6 = data
			}

			list.Append(dst)
		}
	}
	return list
}

func (d *cloudDNSProvider) getRecordMetadata(record *dns.ResourceRecordSet, zone *dns.ManagedZone, project string, recordData string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "record_name", &record.Name)
	schema.AddMetadata(metadata, "record_type", &record.Type)
	schema.AddMetadata(metadata, "record_data", &recordData)

	if record.Ttl != 0 {
		metadata["ttl"] = fmt.Sprintf("%d", record.Ttl)
	}

	metadata["project_id"] = project
	metadata["owner_id"] = project

	if zone != nil {
		schema.AddMetadata(metadata, "zone_name", &zone.Name)
		schema.AddMetadata(metadata, "zone_dns_name", &zone.DnsName)
		schema.AddMetadata(metadata, "zone_description", &zone.Description)
		schema.AddMetadata(metadata, "zone_visibility", &zone.Visibility)

		if zone.Id != 0 {
			metadata["zone_id"] = fmt.Sprintf("%d", zone.Id)
		}
		schema.AddMetadata(metadata, "zone_creation_time", &zone.CreationTime)

		if len(zone.NameServers) > 0 {
			metadata["name_servers"] = strings.Join(zone.NameServers, ",")
			metadata["name_servers_count"] = fmt.Sprintf("%d", len(zone.NameServers))
		}

		if len(zone.Labels) > 0 {
			var labelPairs []string
			for key, value := range zone.Labels {
				labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", key, value))
			}
			metadata["zone_labels"] = strings.Join(labelPairs, ",")
		}

		if zone.PrivateVisibilityConfig != nil {
			metadata["is_private_zone"] = "true"
		} else {
			metadata["is_private_zone"] = "false"
		}
	}

	if len(record.SignatureRrdatas) > 0 {
		metadata["signature_count"] = fmt.Sprintf("%d", len(record.SignatureRrdatas))
	}

	return metadata
}
