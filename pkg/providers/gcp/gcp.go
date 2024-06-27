package gcp

import (
	"context"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	errorutil "github.com/projectdiscovery/utils/errors"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/compute/v1"
	container "google.golang.org/api/container/v1beta1"
	"google.golang.org/api/dns/v1"
)

// Provider is a data provider for gcp API
type Provider struct {
	dns      *dns.Service
	gke      *container.Service
	compute  *compute.Service
	id       string
	projects []string
	services schema.ServiceMap
}

var Services = []string{"dns", "gke", "compute"}

const serviceAccountJSON = "gcp_service_account_key"
const providerName = "gcp"

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

// New creates a new provider client for gcp API
func New(options schema.OptionBlock) (*Provider, error) {
	JSONData, ok := options.GetMetadata(serviceAccountJSON)
	if !ok {
		return nil, errorutil.New("could not get API Key")
	}
	id, _ := options.GetMetadata("id")

	provider := &Provider{id: id}
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
	provider.services = services

	creds, err := register(context.Background(), []byte(JSONData))
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not register gcp service account")
	}
	if services.Has("dns") {
		dnsService, err := dns.NewService(context.Background(), creds)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not create dns service with api key")
		}
		provider.dns = dnsService
	}
	if services.Has("compute") {
		computeService, err := compute.NewService(context.Background(), creds)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not create compute service with api key")
		}
		provider.compute = computeService
	}

	if services.Has("gke") {
		containerService, err := container.NewService(context.Background(), creds)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not create container service with api key")
		}
		provider.gke = containerService
	}

	projects := []string{}
	manager, err := cloudresourcemanager.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not list projects")
	}
	list := manager.Projects.List()
	err = list.Pages(context.Background(), func(resp *cloudresourcemanager.ListProjectsResponse) error {
		for _, project := range resp.Projects {
			projects = append(projects, project.ProjectId)
		}
		return nil
	})
	provider.projects = projects
	return provider, err
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalList := schema.NewResources()

	if p.dns != nil {
		cloudDNSProvider := &cloudDNSProvider{dns: p.dns, id: p.id, projects: p.projects}
		zones, err := cloudDNSProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalList.Merge(zones)
	}

	if p.gke != nil {
		GKEProvider := &gkeProvider{svc: p.gke, id: p.id, projects: p.projects}
		gkeData, err := GKEProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get GKE resources: %s\n", err)
		}
		finalList.Merge(gkeData)
	}

	if p.compute != nil {
		VMProvider := &cloudVMProvider{compute: p.compute, id: p.id, projects: p.projects}
		vmData, err := VMProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalList.Merge(vmData)
	}

	return finalList, nil
}
