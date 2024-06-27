package fastly

import (
	"context"

	"github.com/fastly/go-fastly/v3/fastly"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

//
type serviceProvider struct {
	client *fastly.Client
	id     string
}

// GetResource returns all the resources in the store for a provider.
func (d *serviceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	services, err := d.client.ListServices(nil)
	if err != nil {
		return nil, err
	}

	for _, service := range services {
		sdList, err := d.client.ListServiceDomains(&fastly.ListServiceDomainInput{ID: service.ID})
		if err != nil {
			return nil, err
		}
		for _, domain := range sdList {

			list.Append(&schema.Resource{
				Provider: providerName,
				DNSName:  domain.Name,
				ID:       d.id,
				Service: service.Name,
			})
		}
	}

	return list, nil
}
