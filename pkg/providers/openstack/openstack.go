package openstack

import (
	"context"

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

var Services = []string{"instance", "floatingip", "loadbalancer"}

// Provider is a data provider for Openstack API
type Provider struct {
	id           string
	client       *gophercloud.ServiceClient
	network      *gophercloud.ServiceClient
	loadBalancer *gophercloud.ServiceClient
	services     schema.ServiceMap
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

	services := options.ResolveServices(Services)
	endpointOpts := gophercloud.EndpointOpts{Region: "RegionOne"}
	p := &Provider{id: id, services: services}

	// Compute is optional when Neutron or Octavia can still answer the
	// selected services. A missing Nova endpoint must not fail those.
	if services.Has("instance") {
		client, clientErr := openstack.NewComputeV2(provider, endpointOpts)
		if clientErr != nil && !services.Has("floatingip") && !services.Has("loadbalancer") {
			gologger.Error().Msgf("Couldn't use Openstack region: %s\n", clientErr)
			return nil, clientErr
		}
		if clientErr != nil {
			gologger.Warning().Msgf("Couldn't use Openstack compute: %s\n", clientErr)
		} else {
			p.client = client
		}
	}

	// Neutron and Octavia are optional on many clouds, so a missing catalog
	// entry only disables that service instead of failing the provider.
	if services.Has("floatingip") {
		p.network = optionalClient(openstack.NewNetworkV2(provider, endpointOpts))
	}
	if services.Has("loadbalancer") {
		p.loadBalancer = optionalClient(openstack.NewLoadBalancerV2(provider, endpointOpts))
	}
	return p, nil
}

// optionalClient drops the client on error, since gophercloud returns a
// non-nil but unusable client when the endpoint is not in the catalog.
func optionalClient(client *gophercloud.ServiceClient, err error) *gophercloud.ServiceClient {
	if err != nil {
		gologger.Warning().Msgf("Couldn't use Openstack service: %s\n", err)
		return nil
	}
	return client
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
	var failed error

	// Neutron owns the floatingip service identity. Nova also reports an
	// attached floating address, and the first writer wins deduplication.
	if p.network != nil {
		provider := &floatingIPProvider{id: p.id, client: p.network}
		resources, err := provider.GetResource(ctx)
		failed = absorb(finalResources, resources, err)
	}
	if p.loadBalancer != nil {
		provider := &loadBalancerProvider{id: p.id, client: p.loadBalancer}
		resources, err := provider.GetResource(ctx)
		if err := absorb(finalResources, resources, err); err != nil {
			failed = err
		}
	}
	if p.services.Has("instance") && p.client != nil {
		provider := &instanceProvider{id: p.id, client: p.client}
		resources, err := provider.GetResource(ctx)
		if err := absorb(finalResources, resources, err); err != nil {
			failed = err
		}
	}
	if failed != nil && len(finalResources.Items) == 0 {
		return nil, failed
	}
	if failed != nil {
		gologger.Warning().Msgf("Couldn't list Openstack resources: %s\n", failed)
	}
	return finalResources, nil
}

// absorb keeps a partial listing. An empty failed listing is returned so the
// caller can surface it when nothing else was collected.
func absorb(final *schema.Resources, resources *schema.Resources, err error) error {
	if resources != nil && len(resources.Items) > 0 {
		final.Merge(resources)
		if err != nil {
			gologger.Warning().Msgf("Couldn't list all Openstack resources: %s\n", err)
		}
		return nil
	}
	return err
}
