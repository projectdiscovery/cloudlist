package vercel

import (
	"context"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

var Services = []string{"deployments", "domains"}

// Provider is a data provider for vercel API
type Provider struct {
	id       string
	client   *vercelClient
	services schema.ServiceMap
}

// New creates a new provider client for vercel API
func New(options schema.OptionBlock) (*Provider, error) {
	accessKey, ok := options.GetMetadata(apiAccessToken)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessToken}
	}
	teamID, _ := options.GetMetadata(apiTeamID)

	id, _ := options.GetMetadata("id")
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

	client := newAPIClient(newClientConfig{
		Token:  accessKey,
		Teamid: teamID,
	})
	return &Provider{client: client, id: id, services: services}, nil
}

const providerName = "vercel"

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

const apiAccessToken = "vercel_api_token"
const apiTeamID = "vercel_team_id"

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	projects, err := p.client.ListProjects(ListProjectsRequest{})
	if err != nil {
		return nil, err
	}
	for _, project := range projects.Projects {
		if p.services.Has("deployments") {
			for _, deployment := range project.LatestDeployments {
				finalResources.Append(&schema.Resource{
					Public:   !deployment.Private,
					Provider: providerName,
					Service:  "deployments",
					ID:       p.id,
					DNSName:  deployment.URL,
				})
			}
		}

		for _, target := range project.Targets.Production.Alias {
			if p.services.Has("domains") {
				finalResources.Append(&schema.Resource{
					Public:   true,
					Provider: providerName,
					Service:  "domains",
					ID:       p.id,
					DNSName:  target,
				})
			}
		}
	}
	return finalResources, nil
}
