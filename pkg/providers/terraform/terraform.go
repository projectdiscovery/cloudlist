package terraform

import (
	"context"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"instance"}

const (
	statePathFile = "tf_state_file"
	providerName  = "terraform"
)

// Provider is a data provider for Terraform
type Provider struct {
	id       string
	path     string
	services schema.ServiceMap
}

// New creates a new provider client for Terraform
func New(options schema.OptionBlock) (*Provider, error) {
	StatePathFile, ok := options.GetMetadata(statePathFile)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: statePathFile}
	}
	id, _ := options.GetMetadata("id")

	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	ss, servicesSpecified := options.GetMetadata("services")
	if servicesSpecified {
		for _, s := range strings.Split(ss, ",") {
			if _, ok := supportedServicesMap[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	if !servicesSpecified {
		for _, s := range Services {
			services[s] = struct{}{}
		}
	}
	if es, ok := options.GetMetadata("exclude_services"); ok {
		for _, s := range strings.Split(es, ",") {
			delete(services, strings.TrimSpace(s))
		}
	}
	return &Provider{path: StatePathFile, id: id, services: services}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.services.Keys()
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	if p.services.Has("instance") {
		provider := &instanceProvider{path: p.path, id: p.id}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
