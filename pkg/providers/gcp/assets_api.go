package gcp

import (
	"context"
	"fmt"
	"strings"

	assetpb "cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/utils/errkit"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/storage/v1"
)

// assetInfo stores asset information for bulk metadata fetching
type assetInfo struct {
	asset    *assetpb.Asset
	resource *schema.Resource
}

// parseAssetToResource converts an Asset to a Resource (without extended metadata)
func (p *OrganizationProvider) parseAssetToResource(asset *assetpb.Asset) *schema.Resource {
	if asset == nil || asset.Resource == nil || asset.Resource.Data == nil {
		return nil
	}

	resource := &schema.Resource{
		ID:       p.id,
		Provider: providerName,
		Public:   true,
	}

	data := asset.Resource.Data

	// Parse based on asset type
	switch asset.AssetType {
	case "compute.googleapis.com/Instance":
		resource.Service = "compute"
		p.extractComputeInstanceIPs(data, resource)

	case "compute.googleapis.com/ForwardingRule":
		resource.Service = "compute"
		resource.PublicIPv4 = getStringField(data, "IPAddress", "address")

	case "compute.googleapis.com/GlobalAddress", "compute.googleapis.com/Address":
		resource.Service = "compute"
		resource.PublicIPv4 = getStringField(data, "address")

	case "dns.googleapis.com/ResourceRecordSet":
		resource.Service = "dns"
		p.extractDNSRecordData(data, resource)

	case "storage.googleapis.com/Bucket":
		resource.Service = "s3"
		if name := getStringField(data, "name"); name != "" {
			resource.DNSName = name + ".storage.googleapis.com"
		}

	case "run.googleapis.com/Service":
		resource.Service = "cloud-run"
		resource.DNSName = getNestedStringField(data, "status", "url")

	case "cloudfunctions.googleapis.com/CloudFunction":
		resource.Service = "cloud-function"
		resource.DNSName = getNestedStringField(data, "httpsTrigger", "url")

	case "container.googleapis.com/Cluster":
		resource.Service = "gke"
		resource.DNSName = getStringField(data, "endpoint")

		// GKE basic metadata from asset data (no extra API calls)
		if p.extendedMetadata {
			metadata := make(map[string]string)
			schema.AddMetadata(metadata, "cluster_name", getStringFieldPointer(data, "name"))
			schema.AddMetadata(metadata, "location", getStringFieldPointer(data, "location"))
			schema.AddMetadata(metadata, "status", getStringFieldPointer(data, "status"))
			if nodeCount := data.Fields["currentNodeCount"]; nodeCount != nil {
				metadata["node_count"] = fmt.Sprintf("%.0f", nodeCount.GetNumberValue())
			}
			resource.Metadata = metadata
		}

	case "tpu.googleapis.com/Node":
		resource.Service = "tpu"
		p.extractTPUEndpoint(data, resource)

	case "file.googleapis.com/Instance":
		resource.Service = "filestore"
		p.extractFilestoreIP(data, resource)

	default:
		return nil
	}

	// Only return resources that have IP addresses or DNS names
	if resource.PublicIPv4 == "" && resource.PublicIPv6 == "" && resource.DNSName == "" {
		return nil
	}

	return resource
}

// enrichAssetsWithMetadata fetches extended metadata in bulk for all assets
func (p *OrganizationProvider) enrichAssetsWithMetadata(ctx context.Context, assets []assetInfo) error {
	if !p.extendedMetadata {
		return nil
	}

	// Group assets by type and project
	storageAssets := make(map[string][]assetInfo)  // project -> assets
	computeAssets := make(map[string][]assetInfo)  // project -> assets
	cloudRunAssets := make(map[string][]assetInfo) // project -> assets
	functionAssets := make(map[string][]assetInfo) // project -> assets
	dnsAssets := make(map[string][]assetInfo)      // project -> assets

	for _, ai := range assets {
		switch ai.asset.AssetType {
		case "compute.googleapis.com/Instance":
			project, _, _, err := parseComputeAssetName(ai.asset.Name)
			if err == nil {
				computeAssets[project] = append(computeAssets[project], ai)
			}

		case "storage.googleapis.com/Bucket":
			if name := getStringField(ai.asset.Resource.Data, "name"); name != "" {
				// For buckets, we need to determine the project from the bucket itself
				// We'll handle this in the bulk fetch
				storageAssets["all"] = append(storageAssets["all"], ai)
			}

		case "run.googleapis.com/Service":
			project, _, _, err := parseCloudRunAssetName(ai.asset.Name)
			if err == nil {
				cloudRunAssets[project] = append(cloudRunAssets[project], ai)
			}

		case "cloudfunctions.googleapis.com/CloudFunction":
			project, _, _, err := parseFunctionAssetName(ai.asset.Name)
			if err == nil {
				functionAssets[project] = append(functionAssets[project], ai)
			}

		case "dns.googleapis.com/ResourceRecordSet":
			project, _, _, _, err := parseDNSAssetName(ai.asset.Name)
			if err == nil {
				dnsAssets[project] = append(dnsAssets[project], ai)
			}
		}
	}

	// Bulk fetch metadata for each resource type
	if len(computeAssets) > 0 && p.compute != nil {
		p.bulkFetchComputeMetadata(ctx, computeAssets)
	}

	if len(storageAssets) > 0 && p.storage != nil {
		p.bulkFetchStorageMetadata(ctx, storageAssets)
	}

	if len(cloudRunAssets) > 0 && p.run != nil {
		p.bulkFetchCloudRunMetadata(ctx, cloudRunAssets)
	}

	if len(functionAssets) > 0 && (p.functionsV1 != nil || p.functionsV2 != nil) {
		p.bulkFetchFunctionMetadata(ctx, functionAssets)
	}

	if len(dnsAssets) > 0 && p.dns != nil {
		p.bulkFetchDNSMetadata(ctx, dnsAssets)
	}

	return nil
}

// bulkFetchComputeMetadata fetches metadata for all compute instances at once
func (p *OrganizationProvider) bulkFetchComputeMetadata(ctx context.Context, assetsByProject map[string][]assetInfo) {
	// Create a map for quick lookup: project/zone/instance -> assetInfo
	instanceMap := make(map[string]assetInfo)
	for _, assets := range assetsByProject {
		for _, ai := range assets {
			project, zone, instance, err := parseComputeAssetName(ai.asset.Name)
			if err == nil {
				key := fmt.Sprintf("%s/%s/%s", project, zone, instance)
				instanceMap[key] = ai
			}
		}
	}

	// Fetch all instances using AggregatedList
	for project := range assetsByProject {
		instances := p.compute.Instances.AggregatedList(project)
		err := instances.Pages(ctx, func(ial *compute.InstanceAggregatedList) error {
			for zone, instancesScopedList := range ial.Items {
				zoneShort := extractResourceName(zone)
				for _, instance := range instancesScopedList.Instances {
					key := fmt.Sprintf("%s/%s/%s", project, zoneShort, instance.Name)
					if ai, found := instanceMap[key]; found {
						// Reuse the metadata extraction logic from cloudVMProvider
						vmProvider := &cloudVMProvider{}
						metadata := vmProvider.getInstanceMetadata(instance, project, zoneShort)
						ai.resource.Metadata = metadata
						gologger.Debug().Msgf("Added bulk metadata for compute instance %s", instance.Name)
					}
				}
			}
			return nil
		})
		if err != nil {
			gologger.Warning().Msgf("Could not bulk fetch compute instances for project %s: %s", project, err)
		}
	}
}

// bulkFetchStorageMetadata fetches metadata for all storage buckets at once
func (p *OrganizationProvider) bulkFetchStorageMetadata(ctx context.Context, assetsByProject map[string][]assetInfo) {
	// Create a map for quick lookup: bucketName -> assetInfo
	bucketMap := make(map[string]assetInfo)
	for _, assets := range assetsByProject {
		for _, ai := range assets {
			if name := getStringField(ai.asset.Resource.Data, "name"); name != "" {
				bucketMap[name] = ai
			}
		}
	}

	// Fetch all buckets across all projects
	for _, project := range p.projects {
		bucketsService := p.storage.Buckets.List(project)
		err := bucketsService.Pages(ctx, func(buckets *storage.Buckets) error {
			for _, bucket := range buckets.Items {
				if ai, found := bucketMap[bucket.Name]; found {
					// Reuse the metadata extraction logic from cloudStorageProvider
					storageProvider := &cloudStorageProvider{extendedMetadata: true}
					metadata := storageProvider.getBucketMetadata(bucket)
					ai.resource.Metadata = metadata
					gologger.Debug().Msgf("Added bulk metadata for bucket %s", bucket.Name)
				}
			}
			return nil
		})
		if err != nil {
			gologger.Warning().Msgf("Could not bulk fetch buckets for project %s: %s", project, err)
		}
	}
}

// bulkFetchCloudRunMetadata fetches metadata for all Cloud Run services at once
func (p *OrganizationProvider) bulkFetchCloudRunMetadata(ctx context.Context, assetsByProject map[string][]assetInfo) {
	// Group by project and location
	serviceMap := make(map[string]assetInfo) // project/location/service -> assetInfo
	locationsByProject := make(map[string]map[string]bool)

	for project, assets := range assetsByProject {
		if _, ok := locationsByProject[project]; !ok {
			locationsByProject[project] = make(map[string]bool)
		}
		for _, ai := range assets {
			proj, location, service, err := parseCloudRunAssetName(ai.asset.Name)
			if err == nil {
				key := fmt.Sprintf("%s/%s/%s", proj, location, service)
				serviceMap[key] = ai
				locationsByProject[project][location] = true
			}
		}
	}

	// Fetch services for each project/location combination
	for project, locations := range locationsByProject {
		for location := range locations {
			parent := fmt.Sprintf("projects/%s/locations/%s", project, location)
			services, err := p.run.Projects.Locations.Services.List(parent).Do()
			if err != nil {
				gologger.Warning().Msgf("Could not list Cloud Run services for %s: %s", parent, err)
				continue
			}

			for _, service := range services.Items {
				key := fmt.Sprintf("%s/%s/%s", project, location, service.Metadata.Name)
				if ai, found := serviceMap[key]; found {
					runProvider := &cloudRunProvider{extendedMetadata: true}
					metadata := runProvider.getServiceMetadata(service)
					ai.resource.Metadata = metadata
					gologger.Debug().Msgf("Added bulk metadata for Cloud Run service %s", service.Metadata.Name)
				}
			}
		}
	}
}

// bulkFetchFunctionMetadata fetches metadata for all Cloud Functions at once
func (p *OrganizationProvider) bulkFetchFunctionMetadata(ctx context.Context, assetsByProject map[string][]assetInfo) {
	// Group by project and location
	functionMap := make(map[string]assetInfo) // project/location/function -> assetInfo
	locationsByProject := make(map[string]map[string]bool)

	for project, assets := range assetsByProject {
		if _, ok := locationsByProject[project]; !ok {
			locationsByProject[project] = make(map[string]bool)
		}
		for _, ai := range assets {
			proj, location, function, err := parseFunctionAssetName(ai.asset.Name)
			if err == nil {
				key := fmt.Sprintf("%s/%s/%s", proj, location, function)
				functionMap[key] = ai
				locationsByProject[project][location] = true
			}
		}
	}

	// Try V2 first, then V1
	for project, locations := range locationsByProject {
		for location := range locations {
			parent := fmt.Sprintf("projects/%s/locations/%s", project, location)

			// Try V2 API
			if p.functionsV2 != nil {
				functions, err := p.functionsV2.Projects.Locations.Functions.List(parent).Do()
				if err == nil {
					provider := &cloudFunctionsProvider{extendedMetadata: true}
					for _, function := range functions.Functions {
						parts := strings.Split(function.Name, "/")
						if len(parts) >= 6 {
							functionName := parts[5]
							key := fmt.Sprintf("%s/%s/%s", project, location, functionName)
							if ai, found := functionMap[key]; found {
								metadata := provider.getFunctionV2Metadata(function)
								ai.resource.Metadata = metadata
								gologger.Debug().Msgf("Added bulk V2 metadata for function %s", functionName)
							}
						}
					}
					continue
				}
			}

			// Fall back to V1 API
			if p.functionsV1 != nil {
				functions, err := p.functionsV1.Projects.Locations.Functions.List(parent).Do()
				if err == nil {
					provider := &cloudFunctionsProvider{extendedMetadata: true}
					for _, function := range functions.Functions {
						parts := strings.Split(function.Name, "/")
						if len(parts) >= 6 {
							functionName := parts[5]
							key := fmt.Sprintf("%s/%s/%s", project, location, functionName)
							if ai, found := functionMap[key]; found {
								metadata := provider.getFunctionV1Metadata(function)
								ai.resource.Metadata = metadata
								gologger.Debug().Msgf("Added bulk V1 metadata for function %s", functionName)
							}
						}
					}
				} else {
					gologger.Warning().Msgf("Could not list functions for %s: %s", parent, err)
				}
			}
		}
	}
}

// bulkFetchDNSMetadata fetches metadata for all DNS records at once
func (p *OrganizationProvider) bulkFetchDNSMetadata(ctx context.Context, assetsByProject map[string][]assetInfo) {
	// Group by project and zone
	recordMap := make(map[string]assetInfo) // project/zone/name/type -> assetInfo
	zonesByProject := make(map[string]map[string]bool)

	for project, assets := range assetsByProject {
		if _, ok := zonesByProject[project]; !ok {
			zonesByProject[project] = make(map[string]bool)
		}
		for _, ai := range assets {
			proj, zone, recordName, recordType, err := parseDNSAssetName(ai.asset.Name)
			if err == nil && zone != "" {
				// Use data from asset if name/type not in asset name
				if recordName == "" {
					recordName = getStringField(ai.asset.Resource.Data, "name")
				}
				if recordType == "" {
					recordType = getStringField(ai.asset.Resource.Data, "type")
				}
				key := fmt.Sprintf("%s/%s/%s/%s", proj, zone, recordName, recordType)
				recordMap[key] = ai
				zonesByProject[project][zone] = true
			}
		}
	}

	// Fetch all DNS zones and their records
	dnsProvider := &cloudDNSProvider{extendedMetadata: true}
	for project, zones := range zonesByProject {
		// First get zone details
		managedZones, err := p.dns.ManagedZones.List(project).Do()
		if err != nil {
			gologger.Warning().Msgf("Could not list DNS zones for project %s: %s", project, err)
			continue
		}

		zoneMap := make(map[string]*dns.ManagedZone)
		for _, zone := range managedZones.ManagedZones {
			zoneMap[zone.Name] = zone
		}

		// Now fetch records for each zone
		for zoneName := range zones {
			zone, found := zoneMap[zoneName]
			if !found {
				continue
			}

			recordsCall := p.dns.ResourceRecordSets.List(project, zoneName)
			err := recordsCall.Pages(ctx, func(resp *dns.ResourceRecordSetsListResponse) error {
				for _, record := range resp.Rrsets {
					key := fmt.Sprintf("%s/%s/%s/%s", project, zoneName, record.Name, record.Type)
					if ai, found := recordMap[key]; found {
						var recordData string
						if len(record.Rrdatas) > 0 {
							recordData = record.Rrdatas[0]
						}
						metadata := dnsProvider.getRecordMetadata(record, zone, project, recordData)
						ai.resource.Metadata = metadata
						gologger.Debug().Msgf("Added bulk metadata for DNS record %s", record.Name)
					}
				}
				return nil
			})
			if err != nil {
				gologger.Warning().Msgf("Could not list DNS records for zone %s: %s", zoneName, err)
			}
		}
	}
}

func parseComputeAssetName(assetName string) (project, zone, instance string, err error) {
	if !strings.HasPrefix(assetName, "//compute.googleapis.com/") {
		return "", "", "", errkit.New("invalid compute asset name format")
	}
	path := strings.TrimPrefix(assetName, "//compute.googleapis.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "zones" || parts[4] != "instances" {
		return "", "", "", errkit.New("unexpected compute asset name format")
	}
	return parts[1], parts[3], parts[5], nil
}

// parseFunctionAssetName extracts function details from asset name
// Format: //cloudfunctions.googleapis.com/projects/PROJECT/locations/LOCATION/functions/FUNCTION
func parseFunctionAssetName(assetName string) (project, location, function string, err error) {
	if !strings.HasPrefix(assetName, "//cloudfunctions.googleapis.com/") {
		return "", "", "", errkit.New("invalid function asset name format")
	}

	path := strings.TrimPrefix(assetName, "//cloudfunctions.googleapis.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "functions" {
		return "", "", "", errkit.New("unexpected function asset name format")
	}

	return parts[1], parts[3], parts[5], nil
}

// parseCloudRunAssetName extracts service details from asset name
// Format: //run.googleapis.com/projects/PROJECT/locations/LOCATION/services/SERVICE
func parseCloudRunAssetName(assetName string) (project, location, service string, err error) {
	if !strings.HasPrefix(assetName, "//run.googleapis.com/") {
		return "", "", "", errkit.New("invalid cloud run asset name format")
	}

	path := strings.TrimPrefix(assetName, "//run.googleapis.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "services" {
		return "", "", "", errkit.New("unexpected cloud run asset name format")
	}

	return parts[1], parts[3], parts[5], nil
}

// parseDNSAssetName extracts zone and record set details from asset name
// Format: //dns.googleapis.com/projects/PROJECT/managedZones/ZONE/rrsets/NAME/TYPE
func parseDNSAssetName(assetName string) (project, zone, recordName, recordType string, err error) {
	if !strings.HasPrefix(assetName, "//dns.googleapis.com/") {
		return "", "", "", "", errkit.New("invalid DNS asset name format")
	}

	path := strings.TrimPrefix(assetName, "//dns.googleapis.com/")
	parts := strings.Split(path, "/")

	// Handle both managedZones and rrsets formats
	if len(parts) >= 4 && parts[0] == "projects" && parts[2] == "managedZones" {
		project = parts[1]
		zone = parts[3]
		if len(parts) >= 7 && parts[4] == "rrsets" {
			recordName = parts[5]
			recordType = parts[6]
		}
		return project, zone, recordName, recordType, nil
	}

	return "", "", "", "", errkit.New("unexpected DNS asset name format")
}
