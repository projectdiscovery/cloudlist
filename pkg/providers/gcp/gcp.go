package gcp

import (
	"context"
	"strings"

	asset "cloud.google.com/go/asset/apiv1"
	assetpb "cloud.google.com/go/asset/apiv1/assetpb"
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
	services  schema.ServiceMap
	id        string
	projects  []string
}

// OrganizationProvider is a provider for organization-level GCP Asset API
type OrganizationProvider struct {
	id             string
	organizationID string
	assetClient    *asset.Client
	services       schema.ServiceMap
	projects       []string
}

// Services that provide IP addresses or DNS names only
var Services = []string{
	"dns",            // DNS names, IPv4/IPv6 addresses from DNS records
	"compute",        // IPv4/IPv6 addresses from VM instances
	"gke",            // DNS names and IPs from Kubernetes ingresses
	"cloud-function", // DNS names from function HTTPS URLs
	"cloud-run",      // DNS names from service URLs
	"s3",             // DNS names for storage buckets
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

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.services.Keys()
}

// Resources returns the provider for an resource deployment source using individual services
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	if p.services.Has("dns") {
		dnsProvider := &cloudDNSProvider{
			id:       p.id,
			dns:      p.dns,
			projects: p.projects,
		}
		dnsResources, err := dnsProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get DNS resources: %s\n", err)
		} else {
			finalResources.Merge(dnsResources)
		}
	}

	if p.services.Has("compute") {
		computeProvider := &cloudVMProvider{
			id:       p.id,
			compute:  p.compute,
			projects: p.projects,
		}
		computeResources, err := computeProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get compute resources: %s\n", err)
		} else {
			finalResources.Merge(computeResources)
		}
	}

	if p.services.Has("gke") {
		gkeProvider := &gkeProvider{
			id:       p.id,
			gke:      p.gke,
			projects: p.projects,
		}
		gkeResources, err := gkeProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get gke resources: %s\n", err)
		} else {
			finalResources.Merge(gkeResources)
		}
	}

	if p.services.Has("s3") {
		storageProvider := &cloudStorageProvider{
			id:       p.id,
			storage:  p.storage,
			projects: p.projects,
		}
		storageResources, err := storageProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get storage resources: %s\n", err)
		} else {
			finalResources.Merge(storageResources)
		}
	}

	if p.services.Has("cloud-function") {
		functionProvider := &cloudFunctionsProvider{
			id:        p.id,
			functions: p.functions,
			projects:  p.projects,
		}
		functionResources, err := functionProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get function resources: %s\n", err)
		} else {
			finalResources.Merge(functionResources)
		}
	}

	if p.services.Has("cloud-run") {
		runProvider := &cloudRunProvider{
			id:       p.id,
			run:      p.run,
			projects: p.projects,
		}
		runResources, err := runProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get run resources: %s\n", err)
		} else {
			finalResources.Merge(runResources)
		}
	}

	return finalResources, nil
}

func New(options schema.OptionBlock) (schema.Provider, error) {
	JSONData, ok := options.GetMetadata(serviceAccountJSON)
	if !ok {
		return nil, errorutil.New("could not get API Key")
	}
	id, _ := options.GetMetadata("id")

	gologger.Info().Msgf("Creating GCP provider with id: %s", id)

	// Check if organization_id is present for organization-level discovery
	if orgID, ok := options.GetMetadata("organization_id"); ok {
		gologger.Info().Msgf("Found organization_id: %s, creating OrgProvider", orgID)
		return newOrganizationProvider(options, id, JSONData, orgID)
	}

	gologger.Info().Msgf("Using individual service provider for IP/DNS discovery")
	return newIndividualProvider(options, id, JSONData)
}

// newIndividualProvider creates the original individual service provider
func newIndividualProvider(options schema.OptionBlock, id, JSONData string) (*Provider, error) {
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

	if services.Has("s3") {
		storageService, err := storage.NewService(context.Background(), creds)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not create storage service with api key")
		}
		provider.storage = storageService
	}
	if services.Has("cloud-function") {
		functionsService, err := cloudfunctions.NewService(context.Background(), creds)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not create functions service with api key")
		}
		provider.functions = functionsService
	}

	if services.Has("cloud-run") {
		cloudRunService, err := run.NewService(context.Background(), creds)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("could not create cloud run service with api key")
		}
		provider.run = cloudRunService
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

// Name returns the name of the provider
func (p *OrganizationProvider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *OrganizationProvider) ID() string {
	return p.id
}

// Services returns the provider services
func (p *OrganizationProvider) Services() []string {
	return p.services.Keys()
}

// Resources returns the provider resources using organization-level Cloud Asset Inventory API
func (p *OrganizationProvider) Resources(ctx context.Context) (*schema.Resources, error) {
	gologger.Info().Msgf("OrgProvider.Resources called with organization_id: '%s', projects: %v, services: %v", p.organizationID, p.projects, p.services.Keys())

	parent := "organizations/" + p.organizationID
	gologger.Info().Msgf("Using organization-level discovery with parent: %s", parent)

	finalResources := schema.NewResources()

	// Use Cloud Asset Inventory API to get assets
	if p.services.Has("all") {
		gologger.Info().Msgf("Found 'all' service, starting comprehensive asset discovery")
		allAssets, err := p.getAllAssets(ctx, parent)
		if err != nil {
			gologger.Warning().Msgf("Could not get all assets: %s", err)
		} else {
			finalResources.Merge(allAssets)
		}
	} else {
		// Get assets for specific services
		for _, service := range p.services.Keys() {
			assets, err := p.getAssetsForService(ctx, parent, service)
			if err != nil {
				gologger.Warning().Msgf("Could not get assets for service %s: %s", service, err)
			} else {
				finalResources.Merge(assets)
			}
		}
	}

	return finalResources, nil
}

// getAllAssets gets all assets using the Cloud Asset Inventory API
func (p *OrganizationProvider) getAllAssets(ctx context.Context, parent string) (*schema.Resources, error) {
	gologger.Info().Msgf("Starting Asset API discovery for parent: %s", parent)

	// Define asset types that provide IP addresses or DNS names
	assetTypes := []string{
		"compute.googleapis.com/Instance",
		"compute.googleapis.com/GlobalAddress",
		"compute.googleapis.com/Address",
		"compute.googleapis.com/ForwardingRule",
		"dns.googleapis.com/ManagedZone",
		"dns.googleapis.com/ResourceRecordSet",
		"storage.googleapis.com/Bucket",
		"run.googleapis.com/Service",
		"cloudfunctions.googleapis.com/CloudFunction",
		"container.googleapis.com/Cluster",
		"tpu.googleapis.com/Node",
		"file.googleapis.com/Instance",
	}

	batchResources, err := p.getAssetsForTypes(ctx, parent, assetTypes)
	if err != nil {
		return nil, err
	}

	gologger.Info().Msgf("Asset discovery completed. Found %d total assets", len(batchResources.Items))
	return batchResources, nil
}

// getAssetsForTypes gets assets for specific asset types
func (p *OrganizationProvider) getAssetsForTypes(ctx context.Context, parent string, assetTypes []string) (*schema.Resources, error) {
	req := &assetpb.ListAssetsRequest{
		Parent:      parent,
		AssetTypes:  assetTypes,
		ContentType: assetpb.ContentType_RESOURCE,
		PageSize:    1000,
	}

	resources := schema.NewResources()
	it := p.assetClient.ListAssets(ctx, req)

	for {
		asset, err := it.Next()
		if err != nil {
			if err.Error() == "no more items in iterator" {
				break
			}
			return nil, err
		}

		resource := p.parseAssetToResource(asset)
		if resource != nil {
			resources.Append(resource)
		}
	}

	return resources, nil
}

// getAssetsForService gets assets for a specific service
func (p *OrganizationProvider) getAssetsForService(ctx context.Context, parent string, service string) (*schema.Resources, error) {
	var assetTypes []string

	switch service {
	case "compute":
		assetTypes = []string{"compute.googleapis.com/Instance", "compute.googleapis.com/GlobalAddress", "compute.googleapis.com/Address", "compute.googleapis.com/ForwardingRule"}
	case "dns":
		assetTypes = []string{"dns.googleapis.com/ManagedZone", "dns.googleapis.com/ResourceRecordSet"}
	case "s3":
		assetTypes = []string{"storage.googleapis.com/Bucket"}
	case "cloud-run":
		assetTypes = []string{"run.googleapis.com/Service"}
	case "cloud-function":
		assetTypes = []string{"cloudfunctions.googleapis.com/CloudFunction"}
	case "gke":
		assetTypes = []string{"container.googleapis.com/Cluster"}
	case "tpu":
		assetTypes = []string{"tpu.googleapis.com/Node"}
	case "filestore":
		assetTypes = []string{"file.googleapis.com/Instance"}
	default:
		return schema.NewResources(), nil
	}

	return p.getAssetsForTypes(ctx, parent, assetTypes)
}

// parseAssetToResource converts an Asset to a Resource
func (p *OrganizationProvider) parseAssetToResource(asset *assetpb.Asset) *schema.Resource {
	if asset == nil || asset.Resource == nil {
		return nil
	}

	resource := &schema.Resource{
		ID:       p.id,
		Provider: providerName,
		Public:   true,
	}

	// Parse based on asset type
	switch asset.AssetType {
	case "compute.googleapis.com/Instance":
		resource.Service = "compute"
		// Extract IP from networkInterfaces like the individual API does
		if data := asset.Resource.Data; data != nil {
			if networkInterfaces, ok := data.Fields["networkInterfaces"]; ok {
				if nicList := networkInterfaces.GetListValue(); nicList != nil && len(nicList.Values) > 0 {
					if nicData := nicList.Values[0].GetStructValue(); nicData != nil {
						if accessConfigs, ok := nicData.Fields["accessConfigs"]; ok {
							if configList := accessConfigs.GetListValue(); configList != nil && len(configList.Values) > 0 {
								if configData := configList.Values[0].GetStructValue(); configData != nil {
									if natIP, ok := configData.Fields["natIP"]; ok {
										resource.PublicIPv4 = natIP.GetStringValue()
									}
									if externalIPv6, ok := configData.Fields["externalIpv6"]; ok {
										resource.PublicIPv6 = externalIPv6.GetStringValue()
									}
								}
							}
						}
					}
				}
			}
		}
	case "compute.googleapis.com/ForwardingRule":
		resource.Service = "compute"
		if data := asset.Resource.Data; data != nil {
			if ipAddress, ok := data.Fields["IPAddress"]; ok {
				resource.PublicIPv4 = ipAddress.GetStringValue()
			} else if address, ok := data.Fields["address"]; ok {
				resource.PublicIPv4 = address.GetStringValue()
			}
		}
	case "compute.googleapis.com/GlobalAddress", "compute.googleapis.com/Address":
		resource.Service = "compute"
		if data := asset.Resource.Data; data != nil {
			if address, ok := data.Fields["address"]; ok {
				resource.PublicIPv4 = address.GetStringValue()
			}
		}
	case "dns.googleapis.com/ResourceRecordSet":
		resource.Service = "dns"
		if data := asset.Resource.Data; data != nil {
			if name, ok := data.Fields["name"]; ok {
				resource.DNSName = name.GetStringValue()
			}
			if rrdatas, ok := data.Fields["rrdatas"]; ok {
				if rrdatas.GetListValue() != nil && len(rrdatas.GetListValue().Values) > 0 {
					firstRecord := rrdatas.GetListValue().Values[0].GetStringValue()
					if recordType, ok := data.Fields["type"]; ok {
						switch recordType.GetStringValue() {
						case "A":
							resource.PublicIPv4 = firstRecord
						case "AAAA":
							resource.PublicIPv6 = firstRecord
						}
					}
				}
			}
		}
	case "storage.googleapis.com/Bucket":
		resource.Service = "s3"
		if data := asset.Resource.Data; data != nil {
			if name, ok := data.Fields["name"]; ok {
				resource.DNSName = name.GetStringValue() + ".storage.googleapis.com"
			}
		}
	case "run.googleapis.com/Service":
		resource.Service = "cloud-run"
		if data := asset.Resource.Data; data != nil {
			if status, ok := data.Fields["status"]; ok {
				if statusData := status.GetStructValue(); statusData != nil {
					if url, ok := statusData.Fields["url"]; ok {
						resource.DNSName = url.GetStringValue()
					}
				}
			}
		}
	case "cloudfunctions.googleapis.com/CloudFunction":
		resource.Service = "cloud-function"
		if data := asset.Resource.Data; data != nil {
			if httpsTrigger, ok := data.Fields["httpsTrigger"]; ok {
				if triggerData := httpsTrigger.GetStructValue(); triggerData != nil {
					if url, ok := triggerData.Fields["url"]; ok {
						resource.DNSName = url.GetStringValue()
					}
				}
			}
		}
	case "container.googleapis.com/Cluster":
		resource.Service = "gke"
		if data := asset.Resource.Data; data != nil {
			if endpoint, ok := data.Fields["endpoint"]; ok {
				resource.DNSName = endpoint.GetStringValue()
			}
		}
	case "tpu.googleapis.com/Node":
		resource.Service = "tpu"
		if data := asset.Resource.Data; data != nil {
			if networkEndpoint, ok := data.Fields["networkEndpoint"]; ok {
				if epList := networkEndpoint.GetListValue(); epList != nil && len(epList.Values) > 0 {
					if epData := epList.Values[0].GetStructValue(); epData != nil {
						if ipAddress, ok := epData.Fields["ipAddress"]; ok {
							resource.PublicIPv4 = ipAddress.GetStringValue()
						}
					}
				}
			}
		}
	case "file.googleapis.com/Instance":
		resource.Service = "filestore"
		if data := asset.Resource.Data; data != nil {
			if networks, ok := data.Fields["networks"]; ok {
				if netList := networks.GetListValue(); netList != nil && len(netList.Values) > 0 {
					if netData := netList.Values[0].GetStructValue(); netData != nil {
						if ipAddresses, ok := netData.Fields["ipAddresses"]; ok {
							if ipList := ipAddresses.GetListValue(); ipList != nil && len(ipList.Values) > 0 {
								resource.PublicIPv4 = ipList.Values[0].GetStringValue()
							}
						}
					}
				}
			}
		}
	default:
		return nil
	}

	// Only return resources that have IP addresses or DNS names
	if resource.PublicIPv4 == "" && resource.PublicIPv6 == "" && resource.DNSName == "" {
		return nil
	}

	return resource
}

// newOrganizationProvider creates a new organization-level provider
func newOrganizationProvider(options schema.OptionBlock, id, JSONData, organizationID string) (*OrganizationProvider, error) {
	provider := &OrganizationProvider{
		id:             id,
		organizationID: organizationID,
	}

	// Get all available services for organization-level discovery
	allServices := []string{
		"compute", "dns", "s3", "cloud-run", "cloud-function", "gke", "tpu", "filestore", "all",
	}

	supportedServicesMap := make(map[string]struct{})
	for _, s := range allServices {
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
		// Default to all services for organization-level discovery
		services["all"] = struct{}{}
	}
	provider.services = services

	// Create Asset API client
	creds, err := register(context.Background(), []byte(JSONData))
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not register gcp service account")
	}

	assetClient, err := asset.NewClient(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create asset client")
	}
	provider.assetClient = assetClient

	// Get projects under the organization
	projects := []string{}
	manager, err := cloudresourcemanager.NewService(context.Background(), creds)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not create resource manager")
	}
	list := manager.Projects.List()
	err = list.Pages(context.Background(), func(resp *cloudresourcemanager.ListProjectsResponse) error {
		for _, project := range resp.Projects {
			projects = append(projects, project.ProjectId)
		}
		return nil
	})
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("could not list projects")
	}
	provider.projects = projects

	return provider, nil
}
