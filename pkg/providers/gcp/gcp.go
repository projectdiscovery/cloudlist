package gcp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	asset "cloud.google.com/go/asset/apiv1"
	assetpb "cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/utils/errkit"
	cloudfunctionsv1 "google.golang.org/api/cloudfunctions/v1"
	"google.golang.org/api/cloudfunctions/v2"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/compute/v1"
	container "google.golang.org/api/container/v1beta1"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	run "google.golang.org/api/run/v1"
	"google.golang.org/api/storage/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Provider is a data provider for gcp API
type Provider struct {
	dns              *dns.Service
	gke              *container.Service
	compute          *compute.Service
	storage          *storage.Service
	functions        *cloudfunctions.Service
	functionsV1      *cloudfunctionsv1.Service
	run              *run.APIService
	services         schema.ServiceMap
	id               string
	projects         []string
	extendedMetadata bool
}

// OrganizationProvider is a provider for organization-level GCP Asset API
type OrganizationProvider struct {
	id                    string
	organizationID        string
	assetClient           *asset.Client
	services              schema.ServiceMap
	projects              []string
	projectScope          *projectScope
	extendedMetadata      bool
	readTimeOffsetSeconds int
	compute               *compute.Service // For extended metadata
	functionsV1           *cloudfunctionsv1.Service
	functionsV2           *cloudfunctions.Service
	run                   *run.APIService
	dns                   *dns.Service
	storage               *storage.Service
	gke                   *container.Service
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
	var errs []error

	if p.services.Has("dns") {
		dnsProvider := &cloudDNSProvider{
			id:               p.id,
			dns:              p.dns,
			projects:         p.projects,
			extendedMetadata: p.extendedMetadata,
		}
		dnsResources, err := dnsProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get DNS resources: %s\n", err)
			errs = append(errs, fmt.Errorf("dns: %w", err))
		} else {
			finalResources.Merge(dnsResources)
		}
	}

	if p.services.Has("compute") {
		computeProvider := &cloudVMProvider{
			id:               p.id,
			compute:          p.compute,
			projects:         p.projects,
			extendedMetadata: p.extendedMetadata,
		}
		computeResources, err := computeProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get compute resources: %s\n", err)
			errs = append(errs, fmt.Errorf("compute: %w", err))
		} else {
			finalResources.Merge(computeResources)
		}
	}

	if p.services.Has("gke") {
		gkeProvider := &gkeProvider{
			id:               p.id,
			gke:              p.gke,
			projects:         p.projects,
			extendedMetadata: p.extendedMetadata,
		}
		gkeResources, err := gkeProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get gke resources: %s\n", err)
			errs = append(errs, fmt.Errorf("gke: %w", err))
		} else {
			finalResources.Merge(gkeResources)
		}
	}

	if p.services.Has("s3") {
		storageProvider := &cloudStorageProvider{
			id:               p.id,
			storage:          p.storage,
			projects:         p.projects,
			extendedMetadata: p.extendedMetadata,
		}
		storageResources, err := storageProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get storage resources: %s\n", err)
			errs = append(errs, fmt.Errorf("storage: %w", err))
		} else {
			finalResources.Merge(storageResources)
		}
	}

	if p.services.Has("cloud-function") {
		functionProvider := &cloudFunctionsProvider{
			id:               p.id,
			functions:        p.functions,
			projects:         p.projects,
			extendedMetadata: p.extendedMetadata,
		}
		functionResources, err := functionProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get function resources: %s\n", err)
			errs = append(errs, fmt.Errorf("cloud-function: %w", err))
		} else {
			finalResources.Merge(functionResources)
		}
	}

	if p.services.Has("cloud-run") {
		runProvider := &cloudRunProvider{
			id:               p.id,
			run:              p.run,
			projects:         p.projects,
			extendedMetadata: p.extendedMetadata,
		}
		runResources, err := runProvider.GetResource(ctx)
		if err != nil {
			gologger.Warning().Msgf("Could not get run resources: %s\n", err)
			errs = append(errs, fmt.Errorf("cloud-run: %w", err))
		} else {
			finalResources.Merge(runResources)
		}
	}

	if len(finalResources.Items) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}

	return finalResources, nil
}

func New(options schema.OptionBlock) (schema.Provider, error) {
	JSONData, _ := options.GetMetadata(serviceAccountJSON)
	id, _ := options.GetMetadata("id")

	gologger.Info().Msgf("Creating GCP provider with id: %s", id)

	// Note: gcp_service_account_key is optional
	// Authentication will fall back to Application Default Credentials (ADC) if not provided
	// This works for both traditional and short-lived credential modes

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

	if extendedMetadata, ok := options.GetMetadata("extended_metadata"); ok {
		provider.extendedMetadata = extendedMetadata == "true"
	}

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

	configuredProjects := getProjectIDsFromOptions(options)

	// Extract short-lived credentials configuration
	useShortLived := false
	if val, ok := options.GetMetadata("use_short_lived_credentials"); ok {
		useShortLived = val == "true"
	}

	var targetServiceAccount, sourceCredentials, tokenLifetime string
	if useShortLived {
		targetServiceAccount, _ = options.GetMetadata("service_account_email")
		sourceCredentials, _ = options.GetMetadata("source_credentials")
		tokenLifetime, _ = options.GetMetadata("token_lifetime")

		// Set default token lifetime if not specified
		if tokenLifetime == "" {
			tokenLifetime = "3600s" // 1 hour default
		}

		// Validate required parameters
		if targetServiceAccount == "" {
			return nil, errkit.New("service_account_email is required when use_short_lived_credentials is true")
		}
	}

	// Register credentials with appropriate method
	var creds option.ClientOption
	var err error
	if useShortLived {
		creds, err = registerWithOptions(
			context.Background(),
			[]byte(JSONData),
			true,
			targetServiceAccount,
			sourceCredentials,
			tokenLifetime,
		)
	} else {
		creds, err = register(context.Background(), []byte(JSONData))
	}
	if err != nil {
		return nil, errkit.Wrap(err, "could not register gcp service account")
	}
	if services.Has("dns") {
		dnsService, err := dns.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create dns service with api key")
		}
		provider.dns = dnsService
	}
	if services.Has("compute") {
		computeService, err := compute.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create compute service with api key")
		}
		provider.compute = computeService
	}

	if services.Has("gke") {
		containerService, err := container.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create container service with api key")
		}
		provider.gke = containerService
	}

	if services.Has("s3") {
		storageService, err := storage.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create storage service with api key")
		}
		provider.storage = storageService
	}
	if services.Has("cloud-function") {
		// Initialize v2 service
		functionsService, err := cloudfunctions.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create functions v2 service with api key")
		}
		provider.functions = functionsService

		// Initialize v1 service
		functionsV1Service, err := cloudfunctionsv1.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create functions v1 service with api key")
		}
		provider.functionsV1 = functionsV1Service
	}

	if services.Has("cloud-run") {
		cloudRunService, err := run.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create cloud run service with api key")
		}
		provider.run = cloudRunService
	}

	projects := append([]string{}, configuredProjects...)
	if len(projects) == 0 {
		manager, err := cloudresourcemanager.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not list projects")
		}
		list := manager.Projects.List()
		err = list.Pages(context.Background(), func(resp *cloudresourcemanager.ListProjectsResponse) error {
			for _, project := range resp.Projects {
				projects = append(projects, project.ProjectId)
			}
			return nil
		})
		if err != nil {
			return nil, errkit.Wrap(err, "could not iterate projects")
		}
	}
	if len(projects) == 0 {
		return nil, errkit.New("no projects available for discovery")
	}
	if len(configuredProjects) > 0 {
		gologger.Info().Msgf("Using %d configured GCP project(s) for provider %s", len(projects), id)
	}
	provider.projects = projects
	return provider, nil
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

// Resources returns the provider resources using project-scoped / organization-level Cloud Asset Inventory API
func (p *OrganizationProvider) Resources(ctx context.Context) (*schema.Resources, error) {
	gologger.Info().Msgf("OrgProvider.Resources called with organization_id: '%s', projects: %v, services: %v", p.organizationID, p.projects, p.services.Keys())

	finalResources := schema.NewResources()

	// Use per-project API calls when specific projects are configured
	if p.projectScope != nil && len(p.projectScope.listIDs()) > 0 {
		gologger.Info().Msgf("Using project-scoped discovery for %d configured projects", len(p.projectScope.listIDs()))

		for _, projectID := range p.projectScope.listIDs() {
			parent := "projects/" + projectID
			gologger.Info().Msgf("Fetching assets for project: %s", projectID)

			var projectResources *schema.Resources
			var err error
           // if projects has all, then get all assets
			if p.services.Has("all") {
				projectResources, err = p.getAllAssets(ctx, parent)
				if err != nil {
					gologger.Warning().Msgf("Could not get all assets for project %s: %s", projectID, err)
					continue
				}
			} else {
				projectResources = schema.NewResources()
				for _, service := range p.services.Keys() {
					assets, err := p.getAssetsForService(ctx, parent, service)
					if err != nil {
						gologger.Warning().Msgf("Could not get assets for service %s in project %s: %s", service, projectID, err)
					} else {
						projectResources.Merge(assets)
					}
				}
			}

			finalResources.Merge(projectResources)
		}
	} else {
		// Fallback to organization-level discovery when no specific projects configured
		parent := "organizations/" + p.organizationID
		gologger.Info().Msgf("Using organization-level discovery with parent: %s", parent)
		// Note: When using organization-level discovery, all assets are wanted maybe?
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
	}

	return finalResources, nil
}

// getAllAssets gets all assets using the Cloud Asset Inventory API
func (p *OrganizationProvider) getAllAssets(ctx context.Context, parent string) (*schema.Resources, error) {
	gologger.Info().Msgf("Starting Asset API discovery for parent: %s", parent)
	if p.readTimeOffsetSeconds > 0 {
		readTime := time.Now().Add(-time.Duration(p.readTimeOffsetSeconds) * time.Second).UTC().Format(time.RFC3339)
		gologger.Info().Msgf("Using read time offset of %d seconds (read_time=%s)", p.readTimeOffsetSeconds, readTime)
	}

	var assetTypesGcpListAPI = []string{
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

	// Define asset types that provide IP addresses or DNS names
	batchResources, err := p.getAssetsForTypes(ctx, parent, assetTypesGcpListAPI)
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

	if p.readTimeOffsetSeconds > 0 {
		now := time.Now()
		readTime := now.Add(-time.Duration(p.readTimeOffsetSeconds) * time.Second)
		if readTime.After(now) {
			readTime = now
		}
		req.ReadTime = timestamppb.New(readTime)
	}

	resources := schema.NewResources()
	it := p.assetClient.ListAssets(ctx, req)

	// Collect all assets, with retry on rate limit errors and partial result preservation
	const maxRateLimitRetries = 10
	const rateLimitWait = 90 * time.Second
	var assetInfos []assetInfo
	var lastErr error
	rateLimitRetries := 0

pagination:
	for {
		asset, err := it.Next()
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}

			// On rate limit, wait and resume from where we left off
			if isRateLimitError(err) && rateLimitRetries < maxRateLimitRetries {
				rateLimitRetries++
				pageToken := it.PageInfo().Token

				gologger.Debug().Msgf("Rate limit hit after %d assets, waiting %s before retry (%d/%d)",
					len(assetInfos), rateLimitWait, rateLimitRetries, maxRateLimitRetries)

				select {
				case <-time.After(rateLimitWait):
				case <-ctx.Done():
					lastErr = ctx.Err()
					break pagination
				}

				// Resume with a new iterator using the page token
				req.PageToken = pageToken
				it = p.assetClient.ListAssets(ctx, req)
				continue
			}

			// Non-rate-limit error or max retries exceeded: save error and return partial results
			lastErr = err
			gologger.Debug().Msgf("ListAssets pagination stopped after %d assets: %s", len(assetInfos), err)
			break
		}

		// Note: When using project-scoped API calls, client-side filtering is unnecessary
		// as the API only returns assets from the specified scope (project or organization)
		// For organization-level calls without project_ids config, all assets are wanted anyway
		needsFiltering := strings.HasPrefix(parent, "organizations/") && p.projectScope != nil
		if needsFiltering && !p.projectScope.allowsAsset(asset) {
			continue
		}

		resource := p.parseAssetToResource(asset)
		if resource != nil {
			assetInfos = append(assetInfos, assetInfo{
				asset:    asset,
				resource: resource,
			})
			if len(assetInfos)%25000 == 0 {
				gologger.Debug().Msgf("Progress: %d assets fetched so far", len(assetInfos))
			}
		}
	}

	// If we had an error, propagate cancellations immediately, otherwise return error only if empty.
	if lastErr != nil {
		if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
			return nil, lastErr
		}
		if len(assetInfos) == 0 {
			return nil, lastErr
		}
	}

	// Bulk fetch extended metadata for all collected assets (if requested)
	if p.extendedMetadata && len(assetInfos) > 0 {
		gologger.Info().Msgf("Bulk fetching extended metadata for %d assets", len(assetInfos))
		if err := p.enrichAssetsWithMetadata(ctx, assetInfos); err != nil {
			gologger.Warning().Msgf("Error enriching assets with metadata: %s", err)
		}
	}

	// Append resources *after* enrichment so we include any added metadata
	for _, ai := range assetInfos {
		resources.Append(ai.resource)
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

// newOrganizationProvider creates a new organization-level provider
func newOrganizationProvider(options schema.OptionBlock, id, JSONData, organizationID string) (*OrganizationProvider, error) {
	provider := &OrganizationProvider{
		id:             id,
		organizationID: organizationID,
	}

	configuredProjects := getProjectIDsFromOptions(options)

	// Check for extended metadata flag
	if extendedMetadata, ok := options.GetMetadata("extended_metadata"); ok {
		provider.extendedMetadata = extendedMetadata == "true"
	}
	if offsetStr, ok := options.GetMetadata("read_time_offset_seconds"); ok {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset > 0 {
			provider.readTimeOffsetSeconds = offset
		}
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

	// Extract short-lived credentials configuration
	useShortLived := false
	if val, ok := options.GetMetadata("use_short_lived_credentials"); ok {
		useShortLived = val == "true"
	}

	var targetServiceAccount, sourceCredentials, tokenLifetime string
	if useShortLived {
		targetServiceAccount, _ = options.GetMetadata("service_account_email")
		sourceCredentials, _ = options.GetMetadata("source_credentials")
		tokenLifetime, _ = options.GetMetadata("token_lifetime")

		// Set default token lifetime if not specified
		if tokenLifetime == "" {
			tokenLifetime = "3600s" // 1 hour default
		}

		// Validate required parameters
		if targetServiceAccount == "" {
			return nil, errkit.New("service_account_email is required when use_short_lived_credentials is true")
		}
	}

	// Create Asset API client with appropriate authentication
	var creds option.ClientOption
	var err error
	if useShortLived {
		creds, err = registerWithOptions(
			context.Background(),
			[]byte(JSONData),
			true,
			targetServiceAccount,
			sourceCredentials,
			tokenLifetime,
		)
	} else {
		creds, err = register(context.Background(), []byte(JSONData))
	}
	if err != nil {
		return nil, errkit.Wrap(err, "could not register gcp service account")
	}

	assetClient, err := asset.NewClient(context.Background(), creds)
	if err != nil {
		return nil, errkit.Wrap(err, "could not create asset client")
	}
	provider.assetClient = assetClient

	// Initialize compute service for extended metadata if enabled
	if provider.extendedMetadata {
		computeService, err := compute.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create compute service for extended metadata (missing permissions?): %s", err)
		} else {
			provider.compute = computeService
		}
		functionsV1Service, err := cloudfunctionsv1.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create functions v1 service for extended metadata: %s", err)
		} else {
			provider.functionsV1 = functionsV1Service
		}
		functionsV2Service, err := cloudfunctions.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create functions v2 service for extended metadata: %s", err)
		} else {
			provider.functionsV2 = functionsV2Service
		}
		runService, err := run.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create cloud run service for extended metadata: %s", err)
		} else {
			provider.run = runService
		}
		dnsService, err := dns.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create dns service for extended metadata: %s", err)
		} else {
			provider.dns = dnsService
		}
		storageService, err := storage.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create storage service for extended metadata: %s", err)
		} else {
			provider.storage = storageService
		}
		gkeService, err := container.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create GKE service for extended metadata: %s", err)
		} else {
			provider.gke = gkeService
		}
		gologger.Info().Msgf("Extended metadata enabled for organization discovery")
	}

	// Get projects under the organization
	projects := []string{}
	if len(configuredProjects) > 0 {
		manager, err := cloudresourcemanager.NewService(context.Background(), creds)
		if err != nil {
			return nil, errkit.Wrap(err, "could not create resource manager")
		}
		scope := newProjectScope(configuredProjects)
		if scope == nil {
			return nil, errkit.New("no valid project ids provided in configuration")
		}
		if err := scope.enrichWithProjectNumbers(context.Background(), manager); err != nil {
			gologger.Warning().Msgf("Could not resolve configured project ids: %s", err)
		}
		projects = scope.listIDs()
		provider.projectScope = scope
		if len(projects) == 0 {
			return nil, errkit.New("no valid project ids available after resolution")
		}
		gologger.Info().Msgf("Restricting organization discovery to %d configured project(s)", len(projects))
	} else {
		manager, err := cloudresourcemanager.NewService(context.Background(), creds)
		if err != nil {
			gologger.Warning().Msgf("Could not create resource manager to list projects: %s", err)
		} else {
			list := manager.Projects.List()
			err = list.Pages(context.Background(), func(resp *cloudresourcemanager.ListProjectsResponse) error {
				for _, project := range resp.Projects {
					projects = append(projects, project.ProjectId)
				}
				return nil
			})
			if err != nil {
				gologger.Warning().Msgf("Could not list projects under organization: %s", err)
			}
		}
		if len(projects) == 0 {
			gologger.Info().Msgf("No projects listed, will use organization-level Asset API discovery for org %s", organizationID)
		}
	}
	provider.projects = projects

	return provider, nil
}

// isRateLimitError returns whether the error is a rate limit error
func isRateLimitError(err error) bool {
	s, ok := status.FromError(err)
	if !ok || s.Code() != codes.ResourceExhausted {
		return false
	}
	return true
}

// Helper functions to reduce nesting and improve readability

func getStringField(data *structpb.Struct, fieldNames ...string) string {
	for _, fieldName := range fieldNames {
		if field, ok := data.Fields[fieldName]; ok {
			return field.GetStringValue()
		}
	}
	return ""
}

func getNestedStringField(data *structpb.Struct, parentField, childField string) string {
	if data == nil {
		return ""
	}
	if parent, ok := data.Fields[parentField]; ok {
		if parentStruct := parent.GetStructValue(); parentStruct != nil {
			return getStringField(parentStruct, childField)
		}
	}
	return ""
}

func getStringFieldPointer(data *structpb.Struct, fieldName string) *string {
	if data == nil {
		return nil
	}
	if field, ok := data.Fields[fieldName]; ok {
		value := field.GetStringValue()
		return &value
	}
	return nil
}

func (p *OrganizationProvider) extractComputeInstanceIPs(data *structpb.Struct, resource *schema.Resource) {
	networkInterfaces, ok := data.Fields["networkInterfaces"]
	if !ok {
		return
	}

	nicList := networkInterfaces.GetListValue()
	if nicList == nil || len(nicList.Values) == 0 {
		return
	}

	nicData := nicList.Values[0].GetStructValue()
	if nicData == nil {
		return
	}

	accessConfigs, ok := nicData.Fields["accessConfigs"]
	if !ok {
		return
	}

	configList := accessConfigs.GetListValue()
	if configList == nil || len(configList.Values) == 0 {
		return
	}

	configData := configList.Values[0].GetStructValue()
	if configData != nil {
		resource.PublicIPv4 = getStringField(configData, "natIP")
		resource.PublicIPv6 = getStringField(configData, "externalIpv6")
	}
}

func (p *OrganizationProvider) extractDNSRecordData(data *structpb.Struct, resource *schema.Resource) {
	resource.DNSName = getStringField(data, "name")

	rrdatas, ok := data.Fields["rrdatas"]
	if !ok {
		return
	}

	rrdataList := rrdatas.GetListValue()
	if rrdataList == nil || len(rrdataList.Values) == 0 {
		return
	}

	firstRecord := rrdataList.Values[0].GetStringValue()
	recordType := getStringField(data, "type")

	switch recordType {
	case "A":
		resource.PublicIPv4 = firstRecord
	case "AAAA":
		resource.PublicIPv6 = firstRecord
	}
}

func (p *OrganizationProvider) extractTPUEndpoint(data *structpb.Struct, resource *schema.Resource) {
	networkEndpoint, ok := data.Fields["networkEndpoint"]
	if !ok {
		return
	}

	epList := networkEndpoint.GetListValue()
	if epList == nil || len(epList.Values) == 0 {
		return
	}

	if epData := epList.Values[0].GetStructValue(); epData != nil {
		resource.PublicIPv4 = getStringField(epData, "ipAddress")
	}
}

func (p *OrganizationProvider) extractFilestoreIP(data *structpb.Struct, resource *schema.Resource) {
	networks, ok := data.Fields["networks"]
	if !ok {
		return
	}

	netList := networks.GetListValue()
	if netList == nil || len(netList.Values) == 0 {
		return
	}

	netData := netList.Values[0].GetStructValue()
	if netData == nil {
		return
	}

	ipAddresses, ok := netData.Fields["ipAddresses"]
	if !ok {
		return
	}

	ipList := ipAddresses.GetListValue()
	if ipList != nil && len(ipList.Values) > 0 {
		resource.PublicIPv4 = ipList.Values[0].GetStringValue()
	}
}

// Verify checks if the GCP provider credentials are valid
func (p *Provider) Verify(ctx context.Context) error {
	if len(p.projects) == 0 {
		return errkit.New("no accessible GCP projects found with provided credentials")
	}

	// For extra verification, try a minimal API call on one service
	var err error
	for _, project := range p.projects {
		var success bool
		if p.compute != nil {
			if _, err = p.compute.Regions.List(project).Do(); err == nil {
				success = true
			}
		} else if p.dns != nil {
			if _, err = p.dns.ManagedZones.List(project).Do(); err == nil {
				success = true
			}
		} else if p.storage != nil {
			if _, err = p.storage.Buckets.List(project).Do(); err == nil {
				success = true
			}
		} else if p.functions != nil {
			if _, err = p.functions.Projects.Locations.List(project).Do(); err == nil {
				success = true
			}
		} else if p.functionsV1 != nil {
			if _, err = p.functionsV1.Projects.Locations.List(project).Do(); err == nil {
				success = true
			}
		} else if p.run != nil {
			if _, err = p.run.Projects.Locations.List(project).Do(); err == nil {
				success = true
			}
		} else if p.gke != nil {
			if _, err = p.gke.Projects.Locations.Clusters.List(project).Do(); err == nil {
				success = true
			}
		}
		// For any one service to be successful, we can return nil
		if success {
			return nil
		}
	}
	if err != nil {
		return errkit.Wrap(err, "failed to verify GCP services")
	}
	return errkit.New("no accessible GCP services found with provided credentials")
}

func (p *OrganizationProvider) Verify(ctx context.Context) error {
	var assetTypesGcpListAPI = []string{
		"compute.googleapis.com/Instance",
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

	// For extra verification, try a minimal API call on one service
	iter := p.assetClient.SearchAllResources(ctx, &assetpb.SearchAllResourcesRequest{
		Scope:      "organizations/" + p.organizationID,
		AssetTypes: assetTypesGcpListAPI,
		PageSize:   1,
	})

	_, err := iter.Next()
	if err != nil && !errors.Is(err, iterator.Done) {
		return errkit.Wrap(err, fmt.Sprintf("failed to verify GCP Asset API access for organization %s", p.organizationID))
	}
	return nil
}

func getProjectIDsFromOptions(options schema.OptionBlock) []string {
	raw, ok := options.GetMetadata("project_ids")
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	return splitAndCleanProjectList(raw)
}

func splitAndCleanProjectList(raw string) []string {
	replacer := strings.NewReplacer("\n", ",", "\r", ",", ";", ",")
	normalized := replacer.Replace(raw)
	parts := strings.Split(normalized, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

type projectScope struct {
	allowedIDs     map[string]struct{}
	allowedNumbers map[string]struct{}
	orderedIDs     []string
}

func newProjectScope(projectIDs []string) *projectScope {
	seen := make(map[string]struct{}, len(projectIDs))
	sanitized := make([]string, 0, len(projectIDs))
	for _, id := range projectIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		sanitized = append(sanitized, trimmed)
	}
	if len(sanitized) == 0 {
		return nil
	}
	scope := &projectScope{
		allowedIDs:     make(map[string]struct{}, len(sanitized)),
		allowedNumbers: make(map[string]struct{}),
		orderedIDs:     append([]string{}, sanitized...),
	}
	for _, id := range sanitized {
		scope.allowedIDs[id] = struct{}{}
		if isNumeric(id) {
			scope.allowedNumbers[id] = struct{}{}
		}
	}
	return scope
}

func (ps *projectScope) listIDs() []string {
	if ps == nil {
		return nil
	}
	return append([]string{}, ps.orderedIDs...)
}

func (ps *projectScope) enrichWithProjectNumbers(ctx context.Context, manager *cloudresourcemanager.Service) error {
	if ps == nil || manager == nil {
		return nil
	}
	var firstErr error
	for idx, id := range ps.orderedIDs {
		project, err := manager.Projects.Get(id).Context(ctx).Do()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if project.ProjectId != "" {
			ps.allowedIDs[project.ProjectId] = struct{}{}
			ps.orderedIDs[idx] = project.ProjectId
		}
		if project.ProjectNumber != 0 {
			number := strconv.FormatInt(project.ProjectNumber, 10)
			ps.allowedNumbers[number] = struct{}{}
		}
	}
	return firstErr
}

func (ps *projectScope) allowsAsset(asset *assetpb.Asset) bool {
	if ps == nil {
		return true
	}
	if ps.containsID(extractProjectIDFromAsset(asset)) {
		return true
	}
	if ps.containsNumber(extractProjectNumberFromAsset(asset)) {
		return true
	}
	return false
}

func (ps *projectScope) containsID(id string) bool {
	if id == "" {
		return false
	}
	_, ok := ps.allowedIDs[id]
	return ok
}

func (ps *projectScope) containsNumber(number string) bool {
	if number == "" {
		return false
	}
	_, ok := ps.allowedNumbers[number]
	return ok
}

func extractProjectIDFromAsset(asset *assetpb.Asset) string {
	if asset == nil {
		return ""
	}
	if id := extractProjectToken(asset.GetName()); id != "" && id != "_" {
		return id
	}
	if resource := asset.GetResource(); resource != nil {
		if id := extractProjectToken(resource.Parent); id != "" && id != "_" {
			return id
		}
		if data := resource.Data; data != nil {
			for _, key := range []string{"projectId", "project", "project_id"} {
				if field, ok := data.Fields[key]; ok {
					if value := strings.TrimSpace(field.GetStringValue()); value != "" && value != "_" {
						return value
					}
				}
			}
		}
	}
	return ""
}

func extractProjectNumberFromAsset(asset *assetpb.Asset) string {
	if asset == nil {
		return ""
	}
	if resource := asset.GetResource(); resource != nil {
		if number := extractNumericProjectToken(resource.Parent); number != "" {
			return number
		}
		if data := resource.Data; data != nil {
			if field, ok := data.Fields["projectNumber"]; ok {
				if value := strings.TrimSpace(field.GetStringValue()); value != "" {
					return value
				}
			}
		}
	}
	for _, ancestor := range asset.Ancestors {
		if number := extractNumericProjectToken(ancestor); number != "" {
			return number
		}
	}
	return ""
}

func extractNumericProjectToken(value string) string {
	token := extractProjectToken(value)
	if token == "" {
		return ""
	}
	if !isNumeric(token) {
		return ""
	}
	return token
}

func extractProjectToken(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "//")
	index := strings.Index(trimmed, "projects/")
	if index == -1 {
		return ""
	}
	trimmed = trimmed[index+len("projects/"):]
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		return false
	}
	return true
}
