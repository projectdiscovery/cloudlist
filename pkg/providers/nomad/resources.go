package nomad

import (
	"context"
	"net"
	"strconv"

	"github.com/hashicorp/nomad/api"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// resourceProvider is an resource provider for nomad APIs
type resourceProvider struct {
	id     string
	client *api.Client
}

// GetInstances returns all the instances in the store for a provider.
func (d *resourceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	nodes := d.client.Nodes()
	jobs := d.client.Jobs()
	allocations := d.client.Allocations()

	regions, err := d.client.Regions().List()
	if err != nil {
		return nil, errors.Wrap(err, "could not list nomad regions")
	}
	nodeAddressMap := make(map[string]string)
	for _, region := range regions {
		queryOpts := &api.QueryOptions{Region: region}

		nodeList, _, err := nodes.List(queryOpts)
		if err != nil {
			return nil, errors.Wrap(err, "could not list nodes for nomad region")
		}
		for _, node := range nodeList {
			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				PublicIPv4: node.Address,
				Service:    "nomad_node",
			})
			nodeAddressMap[node.ID] = node.Address
		}

		jobsList, _, err := jobs.List(queryOpts)
		if err != nil {
			return nil, errors.Wrap(err, "could not list jobs for nomad region")
		}
		for _, job := range jobsList {
			allocs, _, err := jobs.Allocations(job.ID, false, queryOpts)
			if err != nil {
				return nil, errors.Wrap(err, "could not list allocation for nomad job")
			}
			for _, alloc := range allocs {
				allocData, _, err := allocations.Info(alloc.ID, queryOpts)
				if err != nil {
					return nil, errors.Wrap(err, "could not get allocation info for nomad")
				}
				nodeAddress := nodeAddressMap[alloc.NodeID]

				if allocData.AllocatedResources == nil {
					continue
				}
				for _, network := range allocData.AllocatedResources.Shared.Networks {
					if !network.HasPorts() {
						continue
					}
					for _, port := range network.ReservedPorts {
						list.Append(&schema.Resource{
							Provider:   providerName,
							Service:    job.Name,
							ID:         d.id,
							PublicIPv4: net.JoinHostPort(nodeAddress, strconv.Itoa(port.Value)),
						})
					}
					for _, port := range network.DynamicPorts {
						list.Append(&schema.Resource{
							Provider:   providerName,
							Service:    job.Name,
							ID:         d.id,
							PublicIPv4: net.JoinHostPort(nodeAddress, strconv.Itoa(port.Value)),
						})
					}
				}
			}
		}
	}
	return list, nil
}
