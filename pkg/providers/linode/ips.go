package linode

import (
	"context"

	"github.com/linode/linodego"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// ipProvider is an IP address provider for linode API
type ipProvider struct {
	id     string
	client *linodego.Client
}

func (d *ipProvider) name() string {
	return "ip"
}

// GetResource returns all public IP addresses on the account, including ones
// not attached to an instance, which ListInstances never returns.
func (d *ipProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	ips, err := d.client.ListIPAddresses(ctx, nil)
	if err != nil {
		return nil, err
	}

	list := schema.NewResources()
	for _, ip := range ips {
		if !ip.Public || ip.Address == "" {
			continue
		}
		resource := &schema.Resource{
			Provider: providerName,
			ID:       d.id,
			Public:   true,
			Service:  d.name(),
		}
		if ip.Type == linodego.IPTypeIPv6 {
			resource.PublicIPv6 = ip.Address
		} else {
			resource.PublicIPv4 = ip.Address
		}
		list.Append(resource)
	}
	return list, nil
}
