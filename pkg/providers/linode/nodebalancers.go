package linode

import (
	"context"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// nodeBalancerProvider is a NodeBalancer provider for linode API
type nodeBalancerProvider struct {
	id     string
	client *linodego.Client
}

func (d *nodeBalancerProvider) name() string {
	return "nodebalancer"
}

// GetResource returns all the NodeBalancer resources for a provider.
func (d *nodeBalancerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	nodeBalancers, err := d.client.ListNodeBalancers(ctx, nil)
	if err != nil {
		return nil, err
	}

	list := schema.NewResources()
	for _, nb := range nodeBalancers {
		list.Append(&schema.Resource{
			Provider:   providerName,
			DNSName:    stringValue(nb.Hostname),
			PublicIPv4: stringValue(nb.IPv4),
			PublicIPv6: stringValue(nb.IPv6),
			ID:         d.id,
			Public:     true,
			Service:    d.name(),
		})
	}
	return list, nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
