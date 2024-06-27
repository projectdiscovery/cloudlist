package cloudflare

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// dnsProvider is a provider for cloudflare dns resources
type dnsProvider struct {
	id     string
	client *cloudflare.API
}

func (d *dnsProvider) name() string {
	return "dns"
}

// GetResource returns all the resources in the store for a provider.
func (d *dnsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	zones, err := d.client.ListZones(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not list zones")
	}
	for _, zone := range zones {
		// Fetch all records for a zone
		recs, _, err := d.client.ListDNSRecords(ctx, cloudflare.ZoneIdentifier(zone.ID), cloudflare.ListDNSRecordsParams{})
		if err != nil {
			return list, errors.Wrap(err, "could not list zones")
		}
		for _, record := range recs {
			if record.Type != "A" && record.Type != "CNAME" && record.Type != "AAAA" {
				continue
			}
			list.Append(&schema.Resource{
				Public:   true,
				Provider: providerName,
				DNSName:  record.Name,
				ID:       d.id,
				Service: d.name(),
			})
			// Skip CNAME records values to discard duplidate data
			if record.Type == "CNAME" {
				continue
			}
			list.Append(&schema.Resource{
				Public:     true,
				Provider:   providerName,
				PublicIPv4: record.Content,
				ID:         d.id,
				Service:   d.name(),
			})
		}
	}
	return list, nil
}
