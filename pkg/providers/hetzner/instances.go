package hetzner

import (
	"context"
	hetzner "github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for Hetzner Cloud API
type instanceProvider struct {
	id     string
	client *hetzner.Client
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the instances in the store for a provider.
func (p *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	servers, err := p.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		if server.PublicNet.IPv4.IP != nil {
			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         p.id,
				PublicIPv4: server.PublicNet.IPv4.IP.String(),
				PublicIPv6: server.PublicNet.IPv6.IP.String(),
				Public:     true,
				Service:    p.name(),
			})
		}
		for _, privateNet := range server.PrivateNet {
			list.Append(&schema.Resource{
				Provider:    providerName,
				ID:          p.id,
				PrivateIpv4: privateNet.IP.String(),
				Service:     p.name(),
			})
		}
	}
	return list, nil
}
