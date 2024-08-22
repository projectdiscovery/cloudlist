package digitalocean

import (
	"context"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"droplet", "app", "instance"}

// Provider is a data provider for digitalocean API
type Provider struct {
	id       string
	client   *godo.Client
	services schema.ServiceMap
}

// New creates a new provider client for digitalocean API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	id, _ := options.GetMetadata("id")

	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := options.GetMetadata("services"); ok {
		for _, s := range strings.Split(ss, ",") {
			if _, ok := supportedServicesMap[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	if len(services) == 0 {
		for _, s := range Services {
			services[s] = struct{}{}
		}
	}
	return &Provider{id: id, client: godo.NewFromToken(token), services: services}, nil
}

const providerName = "digitalocean"

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

const apiKey = "digitalocean_token"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	if p.services.Has("droplet") || p.services.Has("instance") {
		instanceprovider := &instanceProvider{client: p.client, id: p.id}
		instances, err := instanceprovider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(instances)
	}

	if p.services.Has("app") {
		appprovider := &appsProvider{client: p.client, id: p.id}
		apps, err := appprovider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(apps)
	}

	return finalResources, nil
}
