package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	assetpb "cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	errorutil "github.com/projectdiscovery/utils/errors"
	"google.golang.org/api/dns/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// parseAssetToResource converts an Asset to a Resource
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

	marshalledJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		gologger.Debug().Msgf("Could not marshal data: %s", err)
	}
	fmt.Printf("Data: %s", string(marshalledJSON))

	// Parse based on asset type
	switch asset.AssetType {
	case "compute.googleapis.com/Instance":
		resource.Service = "compute"
		p.extractComputeInstanceIPs(data, resource)

		// Fetch extended metadata if enabled and available
		if p.extendedMetadata && p.compute != nil && asset.Name != "" {
			projectID, zone, instanceName, err := parseComputeAssetName(asset.Name)
			if err == nil {
				metadata := p.fetchComputeInstanceExtendedMetadata(projectID, zone, instanceName)
				if metadata != nil {
					resource.Metadata = metadata
					gologger.Debug().Msgf("Added extended metadata for compute instance %s", instanceName)
				}
			} else {
				gologger.Debug().Msgf("Could not parse compute asset name %s: %s", asset.Name, err)
			}
		}

	case "compute.googleapis.com/ForwardingRule":
		resource.Service = "compute"
		resource.PublicIPv4 = getStringField(data, "IPAddress", "address")

	case "compute.googleapis.com/GlobalAddress", "compute.googleapis.com/Address":
		resource.Service = "compute"
		resource.PublicIPv4 = getStringField(data, "address")

	case "dns.googleapis.com/ResourceRecordSet":
		resource.Service = "dns"
		p.extractDNSRecordData(data, resource)

		// Fetch extended metadata
		if p.extendedMetadata && p.dns != nil && asset.Name != "" {
			metadata := p.fetchDNSExtendedMetadata(asset.Name, data)
			if metadata != nil {
				resource.Metadata = metadata
			}
		}

	case "storage.googleapis.com/Bucket":
		resource.Service = "s3"
		if name := getStringField(data, "name"); name != "" {
			resource.DNSName = name + ".storage.googleapis.com"

			// Fetch extended metadata
			if p.extendedMetadata && p.storage != nil {
				metadata := p.fetchStorageExtendedMetadata(name)
				if metadata != nil {
					resource.Metadata = metadata
				}
			}
		}

	case "run.googleapis.com/Service":
		resource.Service = "cloud-run"
		resource.DNSName = getNestedStringField(data, "status", "url")

		// Fetch extended metadata
		if p.extendedMetadata && p.run != nil && asset.Name != "" {
			metadata := p.fetchCloudRunExtendedMetadata(asset.Name)
			if metadata != nil {
				resource.Metadata = metadata
			}
		}

	case "cloudfunctions.googleapis.com/CloudFunction":
		resource.Service = "cloud-function"
		resource.DNSName = getNestedStringField(data, "httpsTrigger", "url")

		// Fetch extended metadata
		if p.extendedMetadata && asset.Name != "" && (p.functionsV1 != nil || p.functionsV2 != nil) {
			metadata := p.fetchFunctionExtendedMetadata(asset.Name)
			if metadata != nil {
				resource.Metadata = metadata
			}
		}

	case "container.googleapis.com/Cluster":
		resource.Service = "gke"
		resource.DNSName = getStringField(data, "endpoint")

		// GKE extended metadata would require additional K8s client setup
		// For now, just extract basic cluster info from asset data
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

func parseComputeAssetName(assetName string) (project, zone, instance string, err error) {
	if !strings.HasPrefix(assetName, "//compute.googleapis.com/") {
		return "", "", "", errorutil.New("invalid compute asset name format")
	}
	path := strings.TrimPrefix(assetName, "//compute.googleapis.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "zones" || parts[4] != "instances" {
		return "", "", "", errorutil.New("unexpected compute asset name format")
	}
	return parts[1], parts[3], parts[5], nil
}

// fetchComputeInstanceExtendedMetadata fetches extended metadata for a compute instance
func (p *OrganizationProvider) fetchComputeInstanceExtendedMetadata(projectID, zone, instanceName string) map[string]string {
	if p.compute == nil || !p.extendedMetadata {
		return nil
	}

	instance, err := p.compute.Instances.Get(projectID, zone, instanceName).Do()
	if err != nil {
		gologger.Debug().Msgf("Could not fetch extended metadata for instance %s: %s", instanceName, err)
		return nil
	}

	// Reuse the metadata extraction logic from cloudVMProvider
	vmProvider := &cloudVMProvider{}
	return vmProvider.getInstanceMetadata(instance, projectID, zone)
}

// parseFunctionAssetName extracts function details from asset name
// Format: //cloudfunctions.googleapis.com/projects/PROJECT/locations/LOCATION/functions/FUNCTION
func parseFunctionAssetName(assetName string) (project, location, function string, err error) {
	if !strings.HasPrefix(assetName, "//cloudfunctions.googleapis.com/") {
		return "", "", "", errorutil.New("invalid function asset name format")
	}

	path := strings.TrimPrefix(assetName, "//cloudfunctions.googleapis.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "functions" {
		return "", "", "", errorutil.New("unexpected function asset name format")
	}

	return parts[1], parts[3], parts[5], nil
}

// fetchFunctionExtendedMetadata fetches extended metadata for a cloud function
func (p *OrganizationProvider) fetchFunctionExtendedMetadata(functionName string) map[string]string {
	if !p.extendedMetadata {
		return nil
	}

	// Try V2 first
	if p.functionsV2 != nil {
		fullName := functionName
		if !strings.HasPrefix(functionName, "projects/") {
			// Reconstruct full name if needed
			project, location, fname, err := parseFunctionAssetName("//cloudfunctions.googleapis.com/" + functionName)
			if err == nil {
				fullName = fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, fname)
			}
		}

		function, err := p.functionsV2.Projects.Locations.Functions.Get(fullName).Do()
		if err == nil {
			provider := &cloudFunctionsProvider{extendedMetadata: true}
			return provider.getFunctionV2Metadata(function)
		}
	}

	// Try V1 if V2 fails
	if p.functionsV1 != nil {
		fullName := functionName
		if !strings.HasPrefix(functionName, "projects/") {
			project, location, fname, err := parseFunctionAssetName("//cloudfunctions.googleapis.com/" + functionName)
			if err == nil {
				fullName = fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, fname)
			}
		}

		function, err := p.functionsV1.Projects.Locations.Functions.Get(fullName).Do()
		if err == nil {
			provider := &cloudFunctionsProvider{extendedMetadata: true}
			return provider.getFunctionV1Metadata(function)
		}
	}

	return nil
}

// parseCloudRunAssetName extracts service details from asset name
// Format: //run.googleapis.com/projects/PROJECT/locations/LOCATION/services/SERVICE
func parseCloudRunAssetName(assetName string) (project, location, service string, err error) {
	if !strings.HasPrefix(assetName, "//run.googleapis.com/") {
		return "", "", "", errorutil.New("invalid cloud run asset name format")
	}

	path := strings.TrimPrefix(assetName, "//run.googleapis.com/")
	parts := strings.Split(path, "/")

	if len(parts) < 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "services" {
		return "", "", "", errorutil.New("unexpected cloud run asset name format")
	}

	return parts[1], parts[3], parts[5], nil
}

// fetchCloudRunExtendedMetadata fetches extended metadata for a Cloud Run service
func (p *OrganizationProvider) fetchCloudRunExtendedMetadata(serviceName string) map[string]string {
	if !p.extendedMetadata || p.run == nil {
		return nil
	}

	fullName := serviceName
	if !strings.HasPrefix(serviceName, "projects/") {
		project, location, sname, err := parseCloudRunAssetName("//run.googleapis.com/" + serviceName)
		if err == nil {
			fullName = fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, sname)
		}
	}

	service, err := p.run.Projects.Locations.Services.Get(fullName).Do()
	if err != nil {
		gologger.Debug().Msgf("Could not fetch extended metadata for Cloud Run service %s: %s", serviceName, err)
		return nil
	}

	provider := &cloudRunProvider{extendedMetadata: true}
	return provider.getServiceMetadata(service)
}

// fetchStorageExtendedMetadata fetches extended metadata for a storage bucket
func (p *OrganizationProvider) fetchStorageExtendedMetadata(bucketName string) map[string]string {
	if !p.extendedMetadata || p.storage == nil {
		return nil
	}

	bucket, err := p.storage.Buckets.Get(bucketName).Do()
	if err != nil {
		gologger.Debug().Msgf("Could not fetch extended metadata for bucket %s: %s", bucketName, err)
		return nil
	}

	provider := &cloudStorageProvider{extendedMetadata: true}
	return provider.getBucketMetadata(bucket)
}

// fetchDNSExtendedMetadata fetches extended metadata for a DNS record
func (p *OrganizationProvider) fetchDNSExtendedMetadata(assetName string, data *structpb.Struct) map[string]string {
	if !p.extendedMetadata || p.dns == nil {
		return nil
	}

	project, zoneName, _, _, err := parseDNSAssetName(assetName)
	if err != nil {
		gologger.Debug().Msgf("Could not parse DNS asset name %s: %s", assetName, err)
		return nil
	}

	zone, err := p.dns.ManagedZones.Get(project, zoneName).Do()
	if err != nil {
		gologger.Debug().Msgf("Could not fetch zone %s: %s", zoneName, err)
		return nil
	}

	recordName := getStringField(data, "name")
	recordType := getStringField(data, "type")
	if recordName == "" || recordType == "" {
		gologger.Debug().Msgf("Missing record name or type in asset data")
		return nil
	}

	var targetRecord *dns.ResourceRecordSet
	recordsCall := p.dns.ResourceRecordSets.List(project, zoneName)
	err = recordsCall.Pages(context.Background(), func(resp *dns.ResourceRecordSetsListResponse) error {
		for _, record := range resp.Rrsets {
			if record.Name == recordName && record.Type == recordType {
				targetRecord = record
				return nil
			}
		}
		return nil
	})

	if err != nil {
		gologger.Debug().Msgf("Could not list resource records: %s", err)
		return nil
	}

	if targetRecord == nil {
		gologger.Debug().Msgf("Could not find record %s of type %s in zone %s", recordName, recordType, zoneName)
		return nil
	}
	var recordData string
	if len(targetRecord.Rrdatas) > 0 {
		recordData = targetRecord.Rrdatas[0]
	}

	provider := &cloudDNSProvider{extendedMetadata: true}
	return provider.getRecordMetadata(targetRecord, zone, project, recordData)
}

// parseDNSAssetName extracts zone and record set details from asset name
// Format: //dns.googleapis.com/projects/PROJECT/managedZones/ZONE/rrsets/NAME/TYPE
func parseDNSAssetName(assetName string) (project, zone, recordName, recordType string, err error) {
	if !strings.HasPrefix(assetName, "//dns.googleapis.com/") {
		return "", "", "", "", errorutil.New("invalid DNS asset name format")
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

	return "", "", "", "", errorutil.New("unexpected DNS asset name format")
}
