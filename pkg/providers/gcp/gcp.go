package gcp

import (
	"context"
	"fmt"
	"strings"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	errorutil "github.com/projectdiscovery/utils/errors"
)

// Provider is a data provider for gcp API
type Provider struct {
	assetClient *asset.Client
	services    schema.ServiceMap
	id          string
	projects    []string
}

var Services = []string{"dns", "gke", "compute", "s3", "cloud-function", "cloud-run"}

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
	organizationId, _ := options.GetMetadata("organization_id")
	id, _ := options.GetMetadata("id")

	provider := &Provider{id: id}
	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := options.GetMetadata("services"); ok {
		for s := range strings.SplitSeq(ss, ",") {
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

	provider.assetClient, err = asset.NewClient(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create asset client with api key")
	}

	projects, err := listProjects(provider.assetClient, fmt.Sprintf("organizations/%s", organizationId))
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not list projects")
	}
	provider.projects = projects
	return provider, err
}

// listProjects uses the Cloud Asset Inventory API to list all accessible projects
func listProjects(assetClient *asset.Client, parent string) ([]string, error) {
	projects := []string{}
	req := &assetpb.ListAssetsRequest{
		Parent:      parent,
		AssetTypes:  []string{"cloudresourcemanager.googleapis.com/Project"},
		ContentType: assetpb.ContentType_RESOURCE,
	}
	it := assetClient.ListAssets(context.Background(), req)
	for {
		asset, err := it.Next()
		if err != nil {
			break
		}
		if asset.Resource != nil && asset.Resource.Data != nil {
			fields := asset.Resource.Data.Fields
			if nameField, ok := fields["projectId"]; ok {
				projectID := nameField.GetStringValue()
				projects = append(projects, projectID)
			}
		}
	}

	return projects, nil
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	if p.services.Has("dns") {
		cloudDNSProvider := &cloudDNSProvider{id: p.id, assetClient: p.assetClient, projects: p.projects}
		zones, err := cloudDNSProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(zones)
	}

	if p.services.Has("gke") {
		GKEProvider := &gkeProvider{id: p.id, assetClient: p.assetClient, projects: p.projects}
		gkeData, err := GKEProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get GKE resources: %s\n", err)
		}
		finalResources.Merge(gkeData)
	}

	if p.services.Has("compute") {
		VMProvider := &cloudVMProvider{id: p.id, assetClient: p.assetClient, projects: p.projects}
		vmData, err := VMProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(vmData)
	}

	if p.services.Has("s3") {
		cloudStorageProvider := &cloudStorageProvider{id: p.id, projects: p.projects, assetClient: p.assetClient}
		storageData, err := cloudStorageProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(storageData)
	}

	if p.services.Has("cloud-function") {
		cloudFunctionsProvider := &cloudFunctionsProvider{id: p.id, assetClient: p.assetClient, projects: p.projects}
		functionsData, err := cloudFunctionsProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(functionsData)
	}

	if p.services.Has("cloud-run") {
		cloudRunProvider := &cloudRunProvider{id: p.id, assetClient: p.assetClient, projects: p.projects}
		cloudRunData, err := cloudRunProvider.GetResource(ctx)
		if err != nil {
			return nil, err
		}
		finalResources.Merge(cloudRunData)
	}
	return finalResources, nil
}

// Verify checks if the GCP provider credentials are valid
func (p *Provider) Verify(ctx context.Context) error {
	if len(p.projects) == 0 {
		return errorutil.New("no accessible GCP projects found with provided credentials")
	}

	// For extra verification, try a minimal API call on one service
	var err error
	for _, project := range p.projects {
		var success bool
		if p.assetClient != nil {
			// Use asset inventory to verify access
			req := &assetpb.ListAssetsRequest{
				Parent:      "projects/" + project,
				PageSize:    1,
				ContentType: assetpb.ContentType_RESOURCE,
			}
			it := p.assetClient.ListAssets(ctx, req)
			_, err = it.Next()
			if err == nil {
				success = true
			}
		}
		// For any one service to be successful, we can return nil
		if success {
			return nil
		}
	}
	if err != nil {
		return errorutil.NewWithErr(err).Msgf("failed to verify GCP services")
	}
	return errorutil.New("no accessible GCP services found with provided credentials")
}
