package azure

import (
	"context"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const (
	tenantID       = `tenant_id`
	clientID       = `client_id`
	clientSecret   = `client_secret`
	subscriptionID = `subscription_id`

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
	clientID, ok := options.GetMetadata(clientID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: clientID}
	}
	clientSecret, ok := options.GetMetadata(clientSecret)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: clientSecret}
	}
	tenantID, ok := options.GetMetadata(tenantID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: tenantID}
	}
	subscriptionID, ok := options.GetMetadata(subscriptionID)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: subscriptionID}
	}

	profile, _ := options.GetMetadata("profile")

	config := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)
	authorizer, err := config.Authorizer()
	if err != nil {
		return nil, err
	}

	return &Provider{Authorizer: authorizer, SubscriptionID: subscriptionID, profile: profile}, nil

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
