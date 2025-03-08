package gcp

import (
	"context"
	"fmt"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/dns/v1"
)

// cloudDNSProvider is a provider for aws Route53 API
type cloudDNSProvider struct {
	id       string
	dns      *dns.Service
	projects []string
}

func (d *cloudDNSProvider) name() string {
	return "dns"
}

// GetResource returns all the resources in the store for a provider.
func (d *cloudDNSProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		dnsZonesService := d.dns.ManagedZones.List(project)
		err := dnsZonesService.Pages(context.Background(), func(zones *dns.ManagedZonesListResponse) error {
			for _, zone := range zones.ManagedZones {
				dnsRecordsService := d.dns.ResourceRecordSets.List(project, zone.Name)
				err := dnsRecordsService.Pages(context.Background(), func(records *dns.ResourceRecordSetsListResponse) error {
					for _, record := range records.Rrsets {
						if record.Type == "A" || record.Type == "AAAA" {
							for _, data := range record.Rrdatas {
								list.Append(&schema.Resource{
									ID:         d.id,
									Provider:   providerName,
									Public:     true,
									DNSName:    fmt.Sprintf("%s.", record.Name),
									PublicIPv4: data,
									Service:    d.name(),
								})
							}
						}
					}
					return nil
				})
				if err != nil {
					return FormatGoogleError(fmt.Errorf("could not get DNS records for zone %s in project %s: %w", zone.Name, project, err))
				}
			}
			return nil
		})
		if err != nil {
			return nil, FormatGoogleError(fmt.Errorf("could not get DNS zones for project %s: %w", project, err))
		}
	}
	return list, nil
}

// parseRecordsForResourceSet parses and returns the records for a resource set
func (d *cloudDNSProvider) parseRecordsForResourceSet(r *dns.ResourceRecordSetsListResponse) *schema.Resources {
	list := schema.NewResources()

	for _, resource := range r.Rrsets {
		if resource.Type != "A" && resource.Type != "CNAME" && resource.Type != "AAAA" {
			continue
		}

		for _, data := range resource.Rrdatas {
			dst := &schema.Resource{
				DNSName:  resource.Name,
				Public:   true,
				ID:       d.id,
				Provider: providerName,
				Service:  d.name(),
			}

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
