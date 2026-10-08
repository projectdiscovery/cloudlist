package openstack

import (
	"context"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/loadbalancer/v2/loadbalancers"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// loadBalancerProvider lists Octavia load balancer VIPs
type loadBalancerProvider struct {
	id     string
	client *gophercloud.ServiceClient
}

func (p *loadBalancerProvider) name() string {
	return "loadbalancer"
}

// GetResource returns all the load balancers in the store for a provider.
func (p *loadBalancerProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	err := loadbalancers.List(p.client, loadbalancers.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		lbs, err := loadbalancers.ExtractLoadBalancers(page)
		if err != nil {
			return false, err
		}

		// The VIP is usually private; a public address in front of it is a
		// floating IP and is reported by the floatingip service.
		for _, lb := range lbs {
			list.Append(&schema.Resource{
				Provider:    providerName,
				ID:          p.id,
				PrivateIpv4: lb.VipAddress,
				Service:     p.name(),
			})
		}
		return true, nil
	})

	if err != nil {
		gologger.Error().Msgf("Couldn't list Openstack load balancers: %s\n", err)
		return nil, err
	}

	return list, nil
}
