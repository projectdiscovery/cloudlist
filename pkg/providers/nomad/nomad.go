package nomad

import (
	"context"
	"net/url"
	"strings"

	"github.com/hashicorp/nomad/api"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"nomad"}

// Provider is a data provider for nomad resources
type Provider struct {
	id       string
	client   *api.Client
	services schema.ServiceMap
}

// New creates a new provider client for nomad resources API
func New(options schema.OptionBlock) (*Provider, error) {
	nomadURL, ok := options.GetMetadata(nomadURL)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: nomadURL}
	}
	parsed, err := url.Parse(nomadURL)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse nomad URL")
	}
	config := api.DefaultConfig()
	config.Address = nomadURL

	if parsed.Scheme == "https" {
		nomadCA, ok := options.GetMetadata(nomadCAFile)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: nomadCAFile}
		}
		nomadCert, ok := options.GetMetadata(nomadCertFile)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: nomadCertFile}
		}
		nomadKey, ok := options.GetMetadata(nomadKeyFile)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: nomadKeyFile}
		}
		config.TLSConfig.CACert = nomadCA
		config.TLSConfig.ClientCert = nomadCert
		config.TLSConfig.ClientKey = nomadKey
		config.TLSConfig.Insecure = true
	}
	if nomadTokenValue, ok := options.GetMetadata(nomadToken); ok && nomadTokenValue != "" {
		config.SecretID = nomadTokenValue
	}
	if nomadHTTPAuthValue, ok := options.GetMetadata(nomadHTTPAuth); ok && nomadHTTPAuthValue != "" {
		var username, password string
		if strings.Contains(nomadHTTPAuthValue, ":") {
			split := strings.SplitN(nomadHTTPAuthValue, ":", 2)
			username = split[0]
			password = split[1]
		} else {
			username = nomadHTTPAuthValue
		}

		config.HttpAuth = &api.HttpBasicAuth{
			Username: username,
			Password: password,
		}
	}
	conn, err := api.NewClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "could not create nomad api client")
	}
	id, _ := options.GetMetadata("id")
	services := make(schema.ServiceMap)
	for _, s := range Services {
		services[s] = struct{}{}
	}
	return &Provider{id: id, client: conn, services: services}, nil
}

const providerName = "nomad"

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

const (
	nomadURL      = "nomad_url"
	nomadCAFile   = "nomad_ca_file"
	nomadCertFile = "nomad_cert_file"
	nomadKeyFile  = "nomad_key_file"
	nomadToken    = "nomad_token"
	nomadHTTPAuth = "nomad_http_auth"
)

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	if p.services.Has("nomad") {
		provider := &resourceProvider{client: p.client, id: p.id}
		if resources, err := provider.GetResource(ctx); err == nil {
			finalResources.Merge(resources)
		}
	}
	return finalResources, nil
}
