package dnssimple

import (
	"context"
	"log"

	"github.com/dnsimple/dnsimple-go/dnsimple"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// perPage is the maximum page size allowed by the DNSimple API.
const perPage = 100

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

	domains, err := d.listDomains(ctx)
	if err != nil {
		return nil, err
	}

	// For each domain, get its zone records
	for _, domain := range domains {
		zoneRecords, err := d.listRecords(ctx, domain.Name)
		if err != nil {
			log.Printf("Could not get records for domain %s: %s\n", domain.Name, err)
			continue
		}

		for _, record := range zoneRecords {
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
			switch record.Type {
			case "A":
				resource.PublicIPv4 = record.Content
			case "AAAA":
				resource.PublicIPv6 = record.Content
			}

			list.Append(resource)
		}
	}

	return list, nil
}

// listDomains returns the domains of the account across all pages.
func (d *dnsProvider) listDomains(ctx context.Context) ([]dnsimple.Domain, error) {
	var domains []dnsimple.Domain
	options := &dnsimple.DomainListOptions{}
	options.PerPage = dnsimple.Int(perPage)
	for page := 1; ; page++ {
		options.Page = dnsimple.Int(page)
		response, err := d.client.Domains.ListDomains(ctx, d.account, options)
		if err != nil {
			return nil, err
		}
		domains = append(domains, response.Data...)
		if response.Pagination == nil || page >= response.Pagination.TotalPages {
			return domains, nil
		}
	}
}

// listRecords returns the records of a zone across all pages.
func (d *dnsProvider) listRecords(ctx context.Context, zone string) ([]dnsimple.ZoneRecord, error) {
	var records []dnsimple.ZoneRecord
	options := &dnsimple.ZoneRecordListOptions{}
	options.PerPage = dnsimple.Int(perPage)
	for page := 1; ; page++ {
		options.Page = dnsimple.Int(page)
		response, err := d.client.Zones.ListRecords(ctx, d.account, zone, options)
		if err != nil {
			return nil, err
		}
		records = append(records, response.Data...)
		if response.Pagination == nil || page >= response.Pagination.TotalPages {
			return records, nil
		}
	}
}
