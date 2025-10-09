package arvancloud

import (
	"context"
	"fmt"

	r1c "git.arvancloud.ir/arvancloud/cdn-go-sdk"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// dnsProvider is a provider for ArvanCloud DNS resources
type dnsProvider struct {
	id     string
	client *r1c.APIClient
}

func (d *dnsProvider) name() string {
	return "dns"
}

// GetResource returns all the resources in the store for a provider.
func (d *dnsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	domains, _, err := d.client.DomainApi.DomainsIndex(ctx).Execute()
	if err != nil {
		return nil, errors.Wrap(err, "could not get domains")
	}

	for _, domain := range domains.GetData() {
		dnsRecords, _, err := d.client.DNSManagementApi.DnsRecordsIndex(ctx, domain.GetId()).Execute()
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("could not get dns records for domain `%s`", domain.GetName()))
		}

		for _, r := range dnsRecords.GetData() {
			// It's for A/AAAA records that can have multiple values
			arrayRecord := r.DnsRecordGenericArrayValue
			if arrayRecord.GetType() != "a" && arrayRecord.GetType() != "aaaa" {
				continue
			}

			for _, record := range arrayRecord.GetValue() {
				if v, ok := record.(map[string]interface{}); ok {
					resource := &schema.Resource{
						Public:   true,
						Provider: providerName,
						DNSName:  fmt.Sprintf("%s.%s", arrayRecord.GetName(), domain.GetName()),
						ID:       d.id,
						Service:  d.name(),
					}

					if arrayRecord.GetType() == "a" {
						resource.PublicIPv4 = v["ip"].(string)
					} else {
						resource.PublicIPv6 = v["ip"].(string)
					}

					list.Append(resource)
				} else {
					return nil, errors.Wrap(err, fmt.Sprintf("could not get ip for `%s` record", arrayRecord.GetName()))
				}
			}

			// It's for normal records with one value
			objectRecord := r.DnsRecordGenericObjectValue
			if objectRecord.GetType() != "cname" {
				continue
			}

			list.Append(&schema.Resource{
				Public:   true,
				Provider: providerName,
				DNSName:  objectRecord.GetName(),
				ID:       d.id,
				Service:  d.name(),
			})
		}
	}

	return list, nil
}
