package scaleway

import (
	"context"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

var Services = []string{"instance"}

// Provider is a data provider for scaleway API
type Provider struct {
	id       string
	client   *scw.Client
	services schema.ServiceMap
}

// New creates a new provider client for scaleway API
func New(options schema.OptionBlock) (*Provider, error) {
	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	accessToken, ok := options.GetMetadata(apiAccessToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessToken}
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

	client, err := scw.NewClient(scw.WithAuth(accessKey, accessToken))
	if err != nil {
		return nil, err
	}
	return &Provider{client: client, id: id, services: services}, nil
}

const providerName = "scw"

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

const apiAccessKey = "scaleway_access_key"
const apiAccessToken = "scaleway_access_token"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	if p.services.Has("instance") {
		provider := &instanceProvider{instanceAPI: instance.NewAPI(p.client), id: p.id}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
