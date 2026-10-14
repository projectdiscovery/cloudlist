package dnssimple

import (
	"context"
	"fmt"

	"github.com/dnsimple/dnsimple-go/dnsimple"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/utils/errkit"
)

// Provider constants
const (
	providerName = "dnssimple"
	apiToken     = "dnssimple_api_token"
)

var Services = []string{"dns"}

// Provider is a data provider for DNSSimple API
type Provider struct {
	id       string
	client   *dnsimple.Client
	services schema.ServiceMap
	account  string
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

// New creates a new provider client for DNSSimple API
func New(options schema.OptionBlock) (*Provider, error) {
	token, ok := options.GetMetadata(apiToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiToken}
	}
	id, _ := options.GetMetadata("id")

	// Set up the client
	client := dnsimple.NewClient(dnsimple.StaticTokenHTTPClient(context.Background(), token))

	// Configure services
	services := options.ResolveServices(Services)

	provider := &Provider{
		id:       id,
		client:   client,
		services: services,
	}

	// Get and store account ID
	whoamiResponse, err := client.Identity.Whoami(context.Background())
	if err != nil {
		return nil, errkit.Wrap(err, "failed to authenticate with DNSSimple")
	}

	if whoamiResponse.Data.Account == nil {
		return nil, errkit.New("no account information found in DNSSimple response")

	}

	provider.account = fmt.Sprintf("%d", whoamiResponse.Data.Account.ID)

	return provider, nil
}

// Resources returns all the resources from the DNSSimple provider
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	if p.services.Has("dns") {
		dnsProvider := &dnsProvider{client: p.client, id: p.id, account: p.account}
		zones, err := dnsProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(zones)
	}

	return finalResources, nil
}
