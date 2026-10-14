package hetzner

import (
	"context"

	hetzner "github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// loadBalancerProvider is a load balancer provider for Hetzner Cloud API
type loadBalancerProvider struct {
	id     string
	client *hetzner.Client
}

func (p *loadBalancerProvider) name() string {
	return "loadbalancer"
}

// GetResource returns all the load balancers in the store for a provider.
func (p *loadBalancerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	loadBalancers, err := p.client.LoadBalancer.All(ctx)
	if err != nil {
		return nil, err
	}

	for _, lb := range loadBalancers {
		if !lb.PublicNet.Enabled {
			continue
		}
		list.Append(&schema.Resource{
			Provider:   providerName,
			ID:         p.id,
			PublicIPv4: ipString(lb.PublicNet.IPv4.IP),
			PublicIPv6: ipString(lb.PublicNet.IPv6.IP),
			Public:     true,
			Service:    p.name(),
		})
	}
	return list, nil
}
