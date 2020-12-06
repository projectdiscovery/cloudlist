package gcp

import (
	"context"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
)

// Provider is a data provider for gcp API
type Provider struct {
	dns      *dns.Service
	profile  string
	projects []string
}

// New creates a new provider client for gcp API
func New(options schema.OptionBlock) (*Provider, error) {
	gcpDNSKey, ok := options.GetMetadata(serviceAccountJSON)
	if !ok {
		return nil, errors.New("could not get API Key")
	}
	profile, _ := options.GetMetadata("profile")

	creds := option.WithCredentialsJSON([]byte(gcpDNSKey))
	dnsService, err := dns.NewService(context.Background(), creds)
	if err != nil {
		return nil, errors.Wrap(err, "could not create dns service with api key")
	}

	projects := []string{}
	manager, err := cloudresourcemanager.NewService(context.Background(), creds)
	if err != nil {
		return nil, errors.Wrap(err, "could not list projects")
	}
	list := manager.Projects.List()
	err = list.Pages(context.Background(), func(resp *cloudresourcemanager.ListProjectsResponse) error {
		for _, project := range resp.Projects {
			projects = append(projects, project.ProjectId)
		}
		return nil
	})
	return &Provider{dns: dnsService, projects: projects, profile: profile}, err
}

const serviceAccountJSON = "gcp_service_account_key"
const providerName = "gcp"

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
	cloudDNSProvider := &cloudDNSProvider{dns: p.dns, profile: p.profile, projects: p.projects}
	zones, err := cloudDNSProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	return zones, nil
}
