package openstack

import (
	"context"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// instanceProvider is an instance provider for Hetzner Cloud API
type instanceProvider struct {
	id     string
	client *gophercloud.ServiceClient
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetResource returns all the instances in the store for a provider.
func (p *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	err := servers.List(p.client, nil).EachPage(func(page pagination.Page) (bool, error) {
		s, err := servers.ExtractServers(page)
		if err != nil {
			return false, err
		}

		for _, server := range s {
			for _, a := range server.Addresses {
				for _, networkAddresses := range a.([]interface{}) {
					address := networkAddresses.(map[string]interface{})
					if address["OS-EXT-IPS:type"] == "floating" {
						list.Append(&schema.Resource{
							Provider:    providerName,
							ID:          p.id,
							PrivateIpv4: address["addr"].(string),
							Service:     p.name(),
						})
					}
				}
			}
		}

		return true, nil
	})

	if err != nil {
		gologger.Error().Msgf("Couldn't list Openstack servers: %s\n", err)
		return nil, err
	}

	return list, nil
}
