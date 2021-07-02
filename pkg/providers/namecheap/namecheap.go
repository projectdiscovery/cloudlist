package namecheap

import (
	"context"

	"github.com/namecheap/go-namecheap-sdk/v2/namecheap"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/naabu/v2/pkg/scan"
)

const (
	userName     = "namecheap_user_name"
	apiKey       = "namecheap_api_key"
	providerName = "namecheap"
)

// Provider is a data provider for NameCheap API
type Provider struct {
	profile string
	client  *namecheap.Client
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

	profile, _ := options.GetMetadata("profile")

	//using naabu to fetch users public ip
	publicIp, err := scan.WhatsMyIP()
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

	return &Provider{profile: profile, client: namecheap.NewClient(&clientOptions)}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ProfileName returns the name of the provider profile
func (p *Provider) ProfileName() string {
	return p.profile
}

// Resources returns the provider for an resource
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	provider := &domainProvider{client: p.client, profile: p.profile}
	return provider.GetResource(ctx)
}
