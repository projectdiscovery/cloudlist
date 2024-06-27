package consul

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// resourceProvider is an resource provider for Consul APIs
type resourceProvider struct {
	id     string
	client *api.Client
}

// GetInstances returns all the instances in the store for a provider.
func (d *resourceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	catalog := d.client.Catalog()
	dcs, err := catalog.Datacenters()
	if err != nil {
		return nil, errors.Wrap(err, "could not list consul datacenters")
	}

	for _, dc := range dcs {
		nodes, _, err := catalog.Nodes(&api.QueryOptions{Datacenter: dc})
		if err != nil {
			return nil, fmt.Errorf("could not list consul nodes for %v: %s", dc, err)
		}
		for _, node := range nodes {
			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				Service:    "consul_node",
				PublicIPv4: node.Address,
			})
		}

		services, _, err := catalog.Services(&api.QueryOptions{Datacenter: dc})
		if err != nil {
			return nil, fmt.Errorf("could not list consul services for %v: %s", dc, err)
		}
		for service, tags := range services {
			serviceCatalog, _, err := catalog.ServiceMultipleTags(service, tags, &api.QueryOptions{
				Datacenter: dc,
			})
			if err != nil {
				return nil, fmt.Errorf("could not get service %v (%v): %s", service, dc, err)
			}
			for _, item := range serviceCatalog {
				var nodeIP string
				if item.ServiceAddress != "" {
					nodeIP = item.ServiceAddress
				} else {
					nodeIP = item.Address
				}
				port := item.ServicePort

				list.Append(&schema.Resource{
					Provider:   providerName,
					ID:         d.id,
					Service:    item.ServiceName,
					PublicIPv4: net.JoinHostPort(nodeIP, strconv.Itoa(port)),
				})
			}
		}
	}
	return list, nil
}
