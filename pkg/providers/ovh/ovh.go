package ovh

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ovh/go-ovh/ovh"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

const providerName = "ovh"

var Services = []string{"dns"}

type Provider struct {
	id       string
	client   *ovh.Client
	services schema.ServiceMap
}

func New(options schema.OptionBlock) (*Provider, error) {
	id, _ := options.GetMetadata("id")

	// service selection
	supported := map[string]struct{}{"dns": {}}
	services := make(schema.ServiceMap)
	ss, servicesSpecified := options.GetMetadata("services")
	if servicesSpecified {
		for _, s := range strings.Split(ss, ",") {
			s = strings.TrimSpace(s)
			if _, ok := supported[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	if !servicesSpecified {
		for _, s := range Services {
			services[s] = struct{}{}
		}
	}
	if es, ok := options.GetMetadata("exclude_services"); ok {
		for _, s := range strings.Split(es, ",") {
			delete(services, strings.TrimSpace(s))
		}
	}

	// OVH endpoint (default ovh-eu)
	endpoint := "ovh-eu"
	if ep, ok := options.GetMetadata("endpoint"); ok && ep != "" {
		endpoint = ep
	}

	// Credentials
	appKey, ok := options.GetMetadata("application_key")
	if !ok || appKey == "" {
		return nil, &schema.ErrNoSuchKey{Name: "application_key"}
	}
	appSecret, ok := options.GetMetadata("application_secret")
	if !ok || appSecret == "" {
		return nil, &schema.ErrNoSuchKey{Name: "application_secret"}
	}
	consumerKey, ok := options.GetMetadata("consumer_key")
	if !ok || consumerKey == "" {
		return nil, &schema.ErrNoSuchKey{Name: "consumer_key"}
	}

	cli, err := ovh.NewClient(endpoint, appKey, appSecret, consumerKey)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	cli.Client = httpClient

	return &Provider{
		id:       id,
		client:   cli,
		services: services,
	}, nil
}

func (p *Provider) Name() string       { return providerName }
func (p *Provider) ID() string         { return p.id }
func (p *Provider) Services() []string { return p.services.Keys() }

func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	out := schema.NewResources()
	if p.services.Has("dns") {
		d := &dnsProvider{id: p.id, client: p.client}
		r, err := d.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		out.Merge(r)
	}
	return out, nil
}
