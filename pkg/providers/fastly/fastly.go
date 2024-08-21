package fastly

import (
	"context"

	"github.com/fastly/go-fastly/v3/fastly"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"fastly"}

const (
	apiKey       = "fastly_api_key"
	providerName = "fastly"
)

// Provider is a data provider for fastly API
type Provider struct {
	client   *fastly.Client
	id       string
	services schema.ServiceMap
}

// New creates a new provider client for fastly API
func New(options schema.OptionBlock) (*Provider, error) {
	apiKey, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, errors.New("could not get API Key")
	}
	id, _ := options.GetMetadata("id")

	client, err := fastly.NewClient(apiKey)
	if err != nil {
		return nil, err
	}
	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}

	services := make(schema.ServiceMap)
	for _, s := range Services {
		services[s] = struct{}{}
	}

	return &Provider{client: client, id: id, services: services}, err
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
	serviceProvider := &serviceProvider{client: p.client, id: p.id}
	if services, err := serviceProvider.GetResource(ctx); err == nil {
		finalResources.Merge(services)
	}
	return finalResources, nil
}
