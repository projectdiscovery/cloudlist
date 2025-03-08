package dnssimple

import (
	"context"
	"strings"

	"github.com/dnsimple/dnsimple-go/dnsimple"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	errorutil "github.com/projectdiscovery/utils/errors"
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

	provider := &Provider{
		id:       id,
		client:   client,
		services: services,
	}

	// Get and store account ID
	whoamiResponse, err := client.Identity.Whoami(context.Background())
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("failed to authenticate with DNSSimple")
	}

	if whoamiResponse.Data.Account != nil {
		provider.account = whoamiResponse.Data.Account.ID
	} else {
		return nil, errorutil.New("no account information found in DNSSimple response")
	}

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
