package digitalocean

import (
	"context"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

var Services = []string{"droplet", "app", "instance", "reservedip", "loadbalancer", "kubernetes"}

// Provider is a data provider for digitalocean API
type Provider struct {
	id               string
	client           *godo.Client
	services         schema.ServiceMap
	extendedMetadata bool
}

// New creates a new provider client for digitalocean API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiKey}
	}
	id, _ := options.GetMetadata("id")

	services := options.ResolveServices(Services)

	// Check for extended metadata option
	extendedMetadata := false
	if em, ok := options.GetMetadata("extended_metadata"); ok {
		extendedMetadata = em == "true"
	}

	return &Provider{
		id:               id,
		client:           godo.NewFromToken(token),
		services:         services,
		extendedMetadata: extendedMetadata,
	}, nil
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
		instanceprovider := &instanceProvider{
			client:           p.client,
			id:               p.id,
			extendedMetadata: p.extendedMetadata,
		}
		instances, err := instanceprovider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(instances)
	}

	if p.services.Has("app") {
		appprovider := &appsProvider{
			client:           p.client,
			id:               p.id,
			extendedMetadata: p.extendedMetadata,
		}
		apps, err := appprovider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(apps)
	}

	// Newer services only warn on failure so a token scoped to the original
	// services (droplet, app) keeps working after upgrade.
	if p.services.Has("reservedip") {
		reservedipprovider := &reservedIPProvider{
			client:           p.client,
			id:               p.id,
			extendedMetadata: p.extendedMetadata,
		}
		reservedIPs, err := reservedipprovider.GetResource(ctx)
		if reservedIPs != nil {
			finalResources.Merge(reservedIPs)
		}
		if err != nil {
			gologger.Warning().Msgf("digitalocean: could not list reserved ips: %s", err)
		}
	}

	if p.services.Has("loadbalancer") {
		loadbalancerprovider := &loadBalancerProvider{
			client:           p.client,
			id:               p.id,
			extendedMetadata: p.extendedMetadata,
		}
		loadBalancers, err := loadbalancerprovider.GetResource(ctx)
		if loadBalancers != nil {
			finalResources.Merge(loadBalancers)
		}
		if err != nil {
			gologger.Warning().Msgf("digitalocean: could not list load balancers: %s", err)
		}
	}

	if p.services.Has("kubernetes") {
		kubernetesprovider := &kubernetesProvider{
			client:           p.client,
			id:               p.id,
			extendedMetadata: p.extendedMetadata,
		}
		clusters, err := kubernetesprovider.GetResource(ctx)
		if clusters != nil {
			finalResources.Merge(clusters)
		}
		if err != nil {
			gologger.Warning().Msgf("digitalocean: could not list kubernetes clusters: %s", err)
		}
	}

	return finalResources, nil
}
