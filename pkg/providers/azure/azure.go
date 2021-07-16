package azure

import (
	"context"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

const (
	profile        = `profile`
	tenantID       = `tenant_id`
	clientID       = `client_id`
	clientSecret   = `client_secret`
	subscriptionID = `subscription_id`
	useCliAuth     = `use_cli_auth`

	providerName = "azure"
)

// Provider is a data provider for Azure API
type Provider struct {
	profile        string
	SubscriptionID string
	Authorizer     autorest.Authorizer
}

// New creates a new provider client for Azure API
func New(options schema.OptionBlock) (*Provider, error) {
	SubscriptionID, ok := options.GetMetadata(subscriptionID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: subscriptionID}
	}

	UseCliAuth, _ := options.GetMetadata(useCliAuth)

	Profile, _ := options.GetMetadata(profile)

	var authorizer autorest.Authorizer
	var err error

	if UseCliAuth == "true" {
		authorizer, err = auth.NewAuthorizerFromCLI()
		if err != nil {
			gologger.Error().Msgf("Couldn't authorize using cli: %s\n", err)
			return nil, err
		}
	} else {
		ClientID, ok := options.GetMetadata(clientID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: clientID}
		}
		ClientSecret, ok := options.GetMetadata(clientSecret)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: clientSecret}
		}
		TenantID, ok := options.GetMetadata(tenantID)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: tenantID}
		}

		config := auth.NewClientCredentialsConfig(ClientID, ClientSecret, TenantID)
		authorizer, err = config.Authorizer()
		if err != nil {
			return nil, err
		}
	}

	return &Provider{Authorizer: authorizer, SubscriptionID: SubscriptionID, profile: Profile}, nil

}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ProfileName returns the name of the provider profile
func (p *Provider) ProfileName() string {
	return p.profile
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &vmProvider{Authorizer: p.Authorizer, SubscriptionID: p.SubscriptionID, profile: p.profile}
	return provider.GetResource(ctx)
}
