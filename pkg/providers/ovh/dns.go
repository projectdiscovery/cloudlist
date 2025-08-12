package ovh

import (
	"context"
	"fmt"
	"strings"

	"github.com/ovh/go-ovh/ovh"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

type dnsProvider struct {
	id     string
	client *ovh.Client
}

func (d *dnsProvider) name() string { return "dns" }

type ovhRecord struct {
	ID        int64  `json:"id"`
	FieldType string `json:"fieldType"`
	SubDomain string `json:"subDomain"`
	Target    string `json:"target"`
	TTL       int    `json:"ttl"`
}

// GetResource returns all DNS resources from OVH
func (d *dnsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	var zones []string
	if err := d.client.GetWithContext(ctx, "/domain/zone", &zones); err != nil {
		return nil, err
	}

	recordTypes := []string{"A", "AAAA", "CNAME"}
	for _, zone := range zones {
		for _, rt := range recordTypes {
			var ids []int64
			path := fmt.Sprintf("/domain/zone/%s/record?fieldType=%s", zone, rt)
			if err := d.client.GetWithContext(ctx, path, &ids); err != nil {
				continue
			}
			for _, id := range ids {
				var rec ovhRecord
				if err := d.client.GetWithContext(ctx, fmt.Sprintf("/domain/zone/%s/record/%d", zone, id), &rec); err != nil {
					continue
				}

				name := strings.TrimSpace(rec.SubDomain)
				var fqdn string
				if name == "" || name == "@" {
					fqdn = zone
				} else {
					fqdn = name + "." + zone
				}

				// Append DNS name (public)
				list.Append(&schema.Resource{
					Public:   true,
					Provider: providerName,
					DNSName:  fqdn,
					ID:       d.id,
					Service:  d.name(),
				})

				// For A/AAAA also append IP resource; skip CNAME target to avoid duplicates
				res := &schema.Resource{
					Public:   true,
					Provider: providerName,
					ID:       d.id,
					Service:  d.name(),
				}

				if strings.ToUpper(rec.FieldType) == "A" {
					res.PublicIPv4 = rec.Target
				} else if strings.ToUpper(rec.FieldType) == "AAAA" {
					res.PublicIPv6 = rec.Target
				}

				list.Append(res)
			}
		}
	}

	return list, nil
}
