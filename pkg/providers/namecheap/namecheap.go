package namecheap

import (
	"context"

	"github.com/namecheap/go-namecheap-sdk/v2/namecheap"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	iputil "github.com/projectdiscovery/utils/ip"
)

const (
	userName     = "namecheap_user_name"
	apiKey       = "namecheap_api_key"
	providerName = "namecheap"
)

var Services = []string{"domain"}

// Provider is a data provider for NameCheap API
type Provider struct {
	id       string
	client   *namecheap.Client
	services schema.ServiceMap
}

// New creates a new provider client for NameCheap API
func New(options schema.OptionBlock) (*Provider, error) {
	apiKey, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	userName, ok := options.GetMetadata(userName)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: userName}
	}

	id, _ := options.GetMetadata("id")

	//using iputil to fetch public ip
	publicIp, err := iputil.WhatsMyIP()
	if err != nil {
		return nil, err
	}

	clientOptions := namecheap.ClientOptions{
		UserName:   userName,
		ApiUser:    userName,
		ApiKey:     apiKey,
		ClientIp:   publicIp,
		UseSandbox: false,
	}

	services := options.ResolveServices(Services)

	return &Provider{id: id, client: namecheap.NewClient(&clientOptions), services: services}, nil
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
	if p.services.Has("domain") {
		provider := &domainProvider{client: p.client, id: p.id}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
