package arvancloud

import (
	"context"
	"strings"

	r1c "git.arvancloud.ir/arvancloud/cdn-go-sdk"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"dns"}

// apiToken is a ArvanCloud user machine token
const apiToken = "api_key"
const providerName = "arvancloud"

// Provider is a data provider for ArvanCloud API
type Provider struct {
	id       string
	client   *r1c.APIClient
	services schema.ServiceMap
}

// New creates a new provider client for ArvanCloud API
func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata("id")
	apiToken, ok := options.GetMetadata(apiToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiToken}
	}

	configuration := r1c.NewConfiguration()
	configuration.AddDefaultHeader("authorization", apiToken)

	// Construct a new API object
	api := r1c.NewAPIClient(configuration)

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

	return &Provider{id: id, client: api, services: services}, nil
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

	if p.services.Has("dns") {
		dnsProvider := &dnsProvider{id: p.id, client: p.client}
		if resources, err := dnsProvider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
