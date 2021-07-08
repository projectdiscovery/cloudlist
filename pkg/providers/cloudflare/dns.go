package cloudflare

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// dnsProvider is a provider for cloudflare dns resources
type dnsProvider struct {
	profile string
	client  *cloudflare.API
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
		recs, err := d.client.DNSRecords(ctx, zone.ID, cloudflare.DNSRecord{})
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
				Profile:  d.profile,
			})
			list.Append(&schema.Resource{
				Public:     true,
				Provider:   providerName,
				PublicIPv4: record.Content,
				Profile:    d.profile,
			})
		}
	}
	return list, nil
}
