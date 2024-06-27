package gcp

import (
	"context"
	"fmt"
	"net/url"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	run "google.golang.org/api/run/v1"
)

type cloudRunProvider struct {
	id       string
	run      *run.APIService
	projects []string
}

func (d *cloudRunProvider) name() string {
	return "cloud-run"
}

// GetResource returns all the Cloud Run resources in the store for a provider.
func (d *cloudRunProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	services, err := d.getServices()
	if err != nil {
		return nil, fmt.Errorf("could not get services: %s", err)
	}
	
	for _, service := range services {
		serviceUrl, _ := url.Parse(service.Status.Url)
		resource := &schema.Resource{
			ID:       d.id,
			Provider: providerName,
			DNSName:  serviceUrl.Hostname(),
			Public:   d.isPublicService(service.Metadata.Name),
			Service:  d.name(),
		}
		list.Append(resource)
	}
	return list, nil
}

func (d *cloudRunProvider) getServices() ([]*run.Service, error) {
	var services []*run.Service
	for _, project := range d.projects {
		locationsService := d.run.Projects.Locations.List(fmt.Sprintf("projects/%s", project))
		locationsResponse, err := locationsService.Do()
		if err != nil {
			continue
		}

		for _, location := range locationsResponse.Locations {
			servicesService := d.run.Projects.Locations.Services.List(location.Name)
			servicesResponse, err := servicesService.Do()
			if err != nil {
				continue
			}
			services = append(services, servicesResponse.Items...)
		}
	}
	return services, nil
}

func (d *cloudRunProvider) isPublicService(serviceName string) bool {
	serviceIAMPolicy, err := d.run.Projects.Locations.Services.GetIamPolicy(serviceName).Do()
	if err == nil {
		for _, binding := range serviceIAMPolicy.Bindings {
			if binding.Role == "roles/run.invoker" {
				for _, member := range binding.Members {
					if member == "allUsers" || member == "allAuthenticatedUsers" {
						return true
					}
				}
			}
		}
	}
	return false
}
