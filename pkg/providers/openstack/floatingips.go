package openstack

import (
	"context"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// floatingIPProvider lists Neutron floating IPs, including ones not attached
// to an instance, which the compute API never returns.
type floatingIPProvider struct {
	id     string
	client *gophercloud.ServiceClient
}

func (p *floatingIPProvider) name() string {
	return "floatingip"
}

// GetResource returns all the floating IPs in the store for a provider.
func (p *floatingIPProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	err := floatingips.List(p.client, floatingips.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		ips, err := floatingips.ExtractFloatingIPs(page)
		if err != nil {
			return false, err
		}

		for _, ip := range ips {
			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         p.id,
				PublicIPv4: ip.FloatingIP,
				Service:    p.name(),
			})
		}
		return true, nil
	})

	if err != nil {
		gologger.Error().Msgf("Couldn't list Openstack floating IPs: %s\n", err)
		return list, err
	}

	return list, nil
}
