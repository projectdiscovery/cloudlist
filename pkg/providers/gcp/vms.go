package gcp

import (
	"context"
	"log"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/compute/v1"
)

type cloudVMProvider struct {
	id       string
	compute  *compute.Service
	projects []string
}

func (d *cloudVMProvider) name() string {
	return "vms"
}

// GetResource returns all the resources in the store for a provider.
func (d *cloudVMProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		instances := d.compute.Instances.AggregatedList(project)
		err := instances.Pages(context.Background(), func(ial *compute.InstanceAggregatedList) error {
			for _, instancesScopedList := range ial.Items {
				for _, instance := range instancesScopedList.Instances {
					instance := instance

					if len(instance.NetworkInterfaces) == 0 {
						continue
					}
					nic := instance.NetworkInterfaces[0]
					if len(nic.AccessConfigs) == 0 {
						continue
					}
					cfg := nic.AccessConfigs[0]

					list.Append(&schema.Resource{
						ID:         d.id,
						Public:     true,
						Provider:   providerName,
						PublicIPv4: cfg.NatIP,
						PublicIPv6: cfg.ExternalIpv6,
						Service:    d.name(),
					})
				}
			}
			return nil
		})
		if err != nil {
			log.Printf("Could not get all instances for project %s: %s\n", project, err)
			continue
		}
	}
	return list, nil
}
