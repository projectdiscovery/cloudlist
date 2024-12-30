package gcp

import (
	"context"
	"log"

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
		zone := d.dns.ManagedZones.List(project)
		err := zone.Pages(context.Background(), func(resp *dns.ManagedZonesListResponse) error {
			for _, z := range resp.ManagedZones {
				resources := d.dns.ResourceRecordSets.List(project, z.Name)
				err := resources.Pages(context.Background(), func(r *dns.ResourceRecordSetsListResponse) error {
					items := d.parseRecordsForResourceSet(r)
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
