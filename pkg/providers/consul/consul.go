package consul

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"consul"}

// Provider is a data provider for consul resources
type Provider struct {
	id       string
	client   *api.Client
	services schema.ServiceMap
}

// New creates a new provider client for consul resources API
func New(options schema.OptionBlock) (*Provider, error) {
	consulURL, ok := options.GetMetadata(consulURL)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: consulURL}
	}

	parsed, err := url.Parse(consulURL)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse consul URL")
	}
	config := api.DefaultConfig()
	if parsed.Scheme == "https" {
		config.Scheme = "https"

		consulCA, ok := options.GetMetadata(consulCAFile)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: consulCAFile}
		}
		consulCert, ok := options.GetMetadata(consulCertFile)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: consulCertFile}
		}
		consulKey, ok := options.GetMetadata(consulKeyFile)
		if !ok {
			return nil, &schema.ErrNoSuchKey{Name: consulKeyFile}
		}

		consulTLSConfig, err := api.SetupTLSConfig(&api.TLSConfig{
			Address:            config.Address,
			CAFile:             consulCA,
			CertFile:           consulCert,
			KeyFile:            consulKey,
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, errors.Wrap(err, "could not create tls consul client")
		}
		config.HttpClient.Transport = &http.Transport{
			TLSClientConfig: consulTLSConfig,
		}
	}
	if consulHTTPToken, ok := options.GetMetadata(consulHTTPToken); ok && consulHTTPToken != "" {
		config.Token = consulHTTPToken
	}
	if consulHTTPAuth, ok := options.GetMetadata(consulHTTPAuth); ok && consulHTTPAuth != "" {
		var username, password string
		if strings.Contains(consulHTTPAuth, ":") {
			split := strings.SplitN(consulHTTPAuth, ":", 2)
			username = split[0]
			password = split[1]
		} else {
			username = consulHTTPAuth
		}

		config.HttpAuth = &api.HttpBasicAuth{
			Username: username,
			Password: password,
		}
	}
	conn, err := api.NewClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "could not create consul api client")
	}
	id, _ := options.GetMetadata("id")

	services := make(schema.ServiceMap)
	for _, s := range Services {
		services[s] = struct{}{}
	}
	return &Provider{id: id, client: conn, services: services}, nil
}

const providerName = "consul"

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
	consulURL       = "consul_url"
	consulHTTPToken = "consul_http_token"
	consulHTTPAuth  = "consul_http_auth"
	consulCAFile    = "consul_ca_file"
	consulCertFile  = "consul_cert_file"
	consulKeyFile   = "consul_key_file"
)

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	provider := &resourceProvider{client: p.client, id: p.id}
	if resources, err := provider.GetResource(ctx); err == nil {
		finalResources.Merge(resources)
	}
	return finalResources, nil
}
