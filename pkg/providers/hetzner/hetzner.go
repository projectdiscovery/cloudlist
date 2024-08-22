package hetzner

import (
	"context"
	"strings"

	hetzner "github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"instance"}

const (
	authToken    = "auth_token"
	providerName = "hetzner"
)

// Provider is a data provider for Hetzner Cloud API
type Provider struct {
	id       string
	client   *hetzner.Client
	services schema.ServiceMap
}

// New creates a new provider client for Hetzner Cloud API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(authToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: authToken}
	}

	id, _ := options.GetMetadata("id")
	opts := hetzner.WithToken(token)

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
	return &Provider{id: id, client: hetzner.NewClient(opts), services: services}, nil
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

// Resources returns the provider for an resource
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	if p.services.Has("instance") {
		provider := &instanceProvider{client: p.client, id: p.id}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
