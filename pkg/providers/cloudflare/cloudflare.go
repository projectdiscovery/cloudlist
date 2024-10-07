package cloudflare

import (
	"context"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"dns"}

// Provider is a data provider for cloudflare API
type Provider struct {
	id       string
	client   *cloudflare.API
	services schema.ServiceMap
}

// New creates a new provider client for cloudflare API
// Here api_token overrides api_key
func New(options schema.OptionBlock) (*Provider, error) {
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

	apiToken, ok := options.GetMetadata(apiToken)
	if ok {
		// Construct a new API object with scoped api token
		api, err := cloudflare.NewWithAPIToken(apiToken)
		if err != nil {
			return nil, err
		}
		return &Provider{id: id, client: api, services: services}, nil
	}

	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	apiEmail, ok := options.GetMetadata(apiEmail)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiEmail}
	}

	// Construct a new API object
	api, err := cloudflare.New(accessKey, apiEmail)
	if err != nil {
		return nil, err
	}

	return &Provider{id: id, client: api, services: services}, nil
}

// apiToken is a cloudflare scoped API token
const apiToken = "api_token"
const apiAccessKey = "api_key"
const apiEmail = "email"
const providerName = "cloudflare"

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
