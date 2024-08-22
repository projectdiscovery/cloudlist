package openstack

import (
	"context"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

const (
	id               = `id`
	identityEndpoint = `identity_endpoint`
	domainName       = `domain_name`
	tenantName       = `tenant_name`
	username         = `username`
	password         = `password`

	providerName = "openstack"
)

var Services = []string{"instance"}

// Provider is a data provider for Openstack API
type Provider struct {
	id       string
	client   *gophercloud.ServiceClient
	services schema.ServiceMap
}

// New creates a new provider client for Openstack API
func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata(id)

	identityEndpoint, ok := options.GetMetadata(identityEndpoint)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: identityEndpoint}
	}

	domainName, ok := options.GetMetadata(domainName)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: domainName}
	}

	tenantName, ok := options.GetMetadata(tenantName)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: tenantName}
	}

	username, ok := options.GetMetadata(username)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: username}
	}

	password, ok := options.GetMetadata(password)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: password}
	}

	opts := gophercloud.AuthOptions{
		IdentityEndpoint: identityEndpoint,
		DomainName:       domainName,
		TenantName:       tenantName,
		Username:         username,
		Password:         password,
	}

	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		gologger.Error().Msgf("Couldn't connect using Openstack credentials: %s\n", err)
		return nil, err
	}

	client, err := openstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})

	if err != nil {
		gologger.Error().Msgf("Couldn't use Openstack region: %s\n", err)
		return nil, err
	}

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
	return &Provider{id: id, client: client, services: services}, nil
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
		provider := &instanceProvider{id: p.id, client: p.client}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
