package gcp

import (
	"context"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	errorutil "github.com/projectdiscovery/utils/errors"
	"google.golang.org/api/cloudfunctions/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/compute/v1"
	container "google.golang.org/api/container/v1beta1"
	"google.golang.org/api/dns/v1"
	run "google.golang.org/api/run/v1"
	"google.golang.org/api/storage/v1"
)

// Provider is a data provider for gcp API
type Provider struct {
	dns       *dns.Service
	gke       *container.Service
	compute   *compute.Service
	storage   *storage.Service
	functions *cloudfunctions.Service
	run       *run.APIService
	id        string
	projects  []string
}

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

// New creates a new provider client for gcp API
func New(options schema.OptionBlock) (*Provider, error) {
	JSONData, ok := options.GetMetadata(serviceAccountJSON)
	if !ok {
		return nil, errorutil.New("could not get API Key")
	}
	id, _ := options.GetMetadata("id")

	creds, err := register(context.Background(), []byte(JSONData))
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not register gcp service account")
	}
	dnsService, err := dns.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create dns service with api key")
	}
	computeService, err := compute.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create compute service with api key")
	}
	containerService, err := container.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create container service with api key")
	}
	storageService, err := storage.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create storage service with api key")
	}
	functionsService, err := cloudfunctions.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create functions service with api key")
	}
	cloudRunService, err := run.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create cloud run service with api key")
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
	return &Provider{dns: dnsService, gke: containerService, projects: projects, id: id, compute: computeService, storage: storageService, functions: functionsService, run: cloudRunService}, err
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalList := schema.NewResources()

	cloudDNSProvider := &cloudDNSProvider{dns: p.dns, id: p.id, projects: p.projects}
	zones, err := cloudDNSProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	finalList.Merge(zones)

	GKEProvider := &gkeProvider{svc: p.gke, id: p.id, projects: p.projects}
	gkeData, err := GKEProvider.GetResource(ctx)
	if err != nil {
		gologger.Warning().Msgf("Could not get GKE resources: %s\n", err)
	}
	finalList.Merge(gkeData)

	VMProvider := &cloudVMProvider{compute: p.compute, id: p.id, projects: p.projects}
	vmData, err := VMProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	finalList.Merge(vmData)

	cloudStorageProvider := &cloudStorageProvider{id: p.id, storage: p.storage, projects: p.projects}
	storageData, err := cloudStorageProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	finalList.Merge(storageData)

	cloudFunctionsProvider := &cloudFunctionsProvider{id: p.id, functions: p.functions, projects: p.projects}
	functionsData, err := cloudFunctionsProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	finalList.Merge(functionsData)

	cloudRunProvider := &cloudRunProvider{id: p.id, run: p.run, projects: p.projects}
	cloudRunData, err := cloudRunProvider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	finalList.Merge(cloudRunData)

	return finalList, nil
}
