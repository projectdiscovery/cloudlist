package dnssimple

import (
	"context"
	"log"

	"github.com/dnsimple/dnsimple-go/dnsimple"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// dnsProvider handles DNS records for DNSSimple
type dnsProvider struct {
	id      string
	client  *dnsimple.Client
	account string
}

func (d *dnsProvider) name() string {
	return "dns"
}

// GetResource returns all DNS resources from DNSSimple
func (d *dnsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	// List all domains
	listOptions := &dnsimple.ListOptions{}
	domains, err := d.client.Domains.ListDomains(ctx, d.account, listOptions)
	if err != nil {
		return nil, err
	}

	// For each domain, get its zone records
	for _, domain := range domains.Data {
		zoneRecords, err := d.client.Zones.ListRecords(ctx, d.account, domain.Name, nil)
		if err != nil {
			log.Printf("Could not get records for domain %s: %s\n", domain.Name, err)
			continue
		}

		for _, record := range zoneRecords.Data {
			// Skip irrelevant record types
			if record.Type != "A" && record.Type != "CNAME" && record.Type != "AAAA" {
				continue
			}

			// Format DNS name properly
			dnsName := record.Name
			if dnsName == "" {
				dnsName = domain.Name
			} else {
				dnsName = dnsName + "." + domain.Name
			}

			resource := &schema.Resource{
				DNSName:  dnsName,
				Public:   true,
				ID:       d.id,
				Provider: providerName,
				Service:  d.name(),
			}

			// Set IP addresses based on record type
			if record.Type == "A" {
				resource.PublicIPv4 = record.Content
			} else if record.Type == "AAAA" {
				resource.PublicIPv6 = record.Content
			}

			list.Append(resource)
		}
	}

	return list, nil
}
