package gcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	cloudfunctionsv1 "google.golang.org/api/cloudfunctions/v1"
	"google.golang.org/api/cloudfunctions/v2"
)

type cloudFunctionsProvider struct {
	id               string
	functions        *cloudfunctions.Service
	functionsV1      *cloudfunctionsv1.Service
	projects         []string
	extendedMetadata bool
}

func (d *cloudFunctionsProvider) name() string {
	return "cloud-function"
}

// GetResource returns all the Cloud Function resources in the store for a provider.
func (d *cloudFunctionsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	// Get v2 functions
	if d.functions != nil {
		functionsV2, err := d.getFunctionsV2()
		if err != nil {
			return nil, fmt.Errorf("could not get v2 functions: %s", err)
		}
		for _, function := range functionsV2 {
			if function == nil || function.Url == "" {
				continue
			}
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getFunctionV2Metadata(function)
			}
			resource := &schema.Resource{
				ID:       d.id,
				Provider: providerName,
				DNSName:  function.Url,
				Public:   d.isPublicFunctionV2(function.Name),
				Service:  "cloud-function-v2",
				Metadata: metadata,
			}
			list.Append(resource)
		}
	}

	// Get v1 functions
	if d.functionsV1 != nil {
		functionsV1, err := d.getFunctionsV1()
		if err != nil {
			return nil, fmt.Errorf("could not get v1 functions: %s", err)
		}
		for _, function := range functionsV1 {
			if function == nil || function.HttpsTrigger == nil || function.HttpsTrigger.Url == "" {
				continue
			}
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getFunctionV1Metadata(function)
			}
			resource := &schema.Resource{
				ID:       d.id,
				Provider: providerName,
				DNSName:  function.HttpsTrigger.Url,
				Public:   d.isPublicFunctionV1(function.Name),
				Service:  "cloud-function-v1",
				Metadata: metadata,
			}
			list.Append(resource)
		}
	}

	return list, nil
}

func (d *cloudFunctionsProvider) getFunctionsV2() ([]*cloudfunctions.Function, error) {
	var functions []*cloudfunctions.Function
	for _, project := range d.projects {
		functionsService := d.functions.Projects.Locations.Functions.List(fmt.Sprintf("projects/%s/locations/-", project))
		_ = functionsService.Pages(context.Background(), func(fal *cloudfunctions.ListFunctionsResponse) error {
			functions = append(functions, fal.Functions...)
			return nil
		})
	}
	return functions, nil
}

func (d *cloudFunctionsProvider) getFunctionsV1() ([]*cloudfunctionsv1.CloudFunction, error) {
	var functions []*cloudfunctionsv1.CloudFunction
	for _, project := range d.projects {
		functionsService := d.functionsV1.Projects.Locations.Functions.List(fmt.Sprintf("projects/%s/locations/-", project))
		_ = functionsService.Pages(context.Background(), func(fal *cloudfunctionsv1.ListFunctionsResponse) error {
			functions = append(functions, fal.Functions...)
			return nil
		})
	}
	return functions, nil
}

func (d *cloudFunctionsProvider) isPublicFunctionV2(functionName string) bool {
	functionIAMPolicy, err := d.functions.Projects.Locations.Functions.GetIamPolicy(functionName).Do()
	if err == nil {
		for _, binding := range functionIAMPolicy.Bindings {
			if binding.Role == "roles/cloudfunctions.invoker" {
				for _, member := range binding.Members {
					if member == "allUsers" || member == "allAuthenticatedUsers" {
						return true
					}
				}
			}
		}
	}
	return false
}

func (d *cloudFunctionsProvider) isPublicFunctionV1(functionName string) bool {
	fullName := functionName
	if !strings.HasPrefix(functionName, "projects/") {
		return false
	}

	functionIAMPolicy, err := d.functionsV1.Projects.Locations.Functions.GetIamPolicy(fullName).Do()
	if err == nil {
		for _, binding := range functionIAMPolicy.Bindings {
			if binding.Role == "roles/cloudfunctions.invoker" {
				for _, member := range binding.Members {
					if member == "allUsers" || member == "allAuthenticatedUsers" {
						return true
					}
				}
			}
		}
	}
	return false
}

func (d *cloudFunctionsProvider) getFunctionV2Metadata(function *cloudfunctions.Function) map[string]string {
	metadata := make(map[string]string)

	// Basic function information
	schema.AddMetadata(metadata, "function_name", &function.Name)

	schema.AddMetadata(metadata, "description", &function.Description)
	schema.AddMetadata(metadata, "state", &function.State)
	schema.AddMetadata(metadata, "environment", &function.Environment)
	schema.AddMetadata(metadata, "create_time", &function.CreateTime)
	schema.AddMetadata(metadata, "update_time", &function.UpdateTime)
	schema.AddMetadata(metadata, "kms_key_name", &function.KmsKeyName)

	// Service configuration details
	if function.ServiceConfig != nil {
		schema.AddMetadata(metadata, "service_name", &function.ServiceConfig.Service)
		schema.AddMetadata(metadata, "service_account", &function.ServiceConfig.ServiceAccountEmail)
		schema.AddMetadata(metadata, "available_memory", &function.ServiceConfig.AvailableMemory)

		schema.AddMetadata(metadata, "ingress_settings", &function.ServiceConfig.IngressSettings)
		schema.AddMetadata(metadata, "vpc_connector", &function.ServiceConfig.VpcConnector)
		schema.AddMetadata(metadata, "vpc_connector_egress_settings", &function.ServiceConfig.VpcConnectorEgressSettings)
	}
	if function.BuildConfig != nil {
		schema.AddMetadata(metadata, "runtime", &function.BuildConfig.Runtime)
		schema.AddMetadata(metadata, "entry_point", &function.BuildConfig.EntryPoint)
		schema.AddMetadata(metadata, "build_service_account", &function.BuildConfig.ServiceAccount)
	}
	if function.EventTrigger != nil {
		schema.AddMetadata(metadata, "event_type", &function.EventTrigger.EventType)
		schema.AddMetadata(metadata, "pubsub_topic", &function.EventTrigger.PubsubTopic)
		schema.AddMetadata(metadata, "trigger_service_account", &function.EventTrigger.ServiceAccountEmail)
	}

	if len(function.Labels) > 0 {
		var labelPairs []string
		for k, v := range function.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
		}
		labelString := strings.Join(labelPairs, ",")
		schema.AddMetadata(metadata, "labels", &labelString)
	}

	return metadata
}

func (d *cloudFunctionsProvider) getFunctionV1Metadata(function *cloudfunctionsv1.CloudFunction) map[string]string {
	metadata := make(map[string]string)

	// Basic function information
	schema.AddMetadata(metadata, "function_name", &function.Name)

	schema.AddMetadata(metadata, "description", &function.Description)
	schema.AddMetadata(metadata, "status", &function.Status)
	schema.AddMetadata(metadata, "entry_point", &function.EntryPoint)
	schema.AddMetadata(metadata, "runtime", &function.Runtime)
	schema.AddMetadata(metadata, "timeout", &function.Timeout)

	if function.AvailableMemoryMb > 0 {
		schema.AddMetadataInt(metadata, "available_memory_mb", int(function.AvailableMemoryMb))
	}

	schema.AddMetadata(metadata, "service_account", &function.ServiceAccountEmail)
	schema.AddMetadata(metadata, "update_time", &function.UpdateTime)

	if function.VersionId > 0 {
		schema.AddMetadataInt(metadata, "version_id", int(function.VersionId))
	}

	if function.MaxInstances > 0 {
		schema.AddMetadataInt(metadata, "max_instances", int(function.MaxInstances))
	}

	if function.MinInstances > 0 {
		schema.AddMetadataInt(metadata, "min_instances", int(function.MinInstances))
	}

	schema.AddMetadata(metadata, "build_id", &function.BuildId)
	schema.AddMetadata(metadata, "build_name", &function.BuildName)
	schema.AddMetadata(metadata, "kms_key_name", &function.KmsKeyName)
	schema.AddMetadata(metadata, "vpc_connector", &function.VpcConnector)
	schema.AddMetadata(metadata, "vpc_connector_egress_settings", &function.VpcConnectorEgressSettings)
	schema.AddMetadata(metadata, "ingress_settings", &function.IngressSettings)
	schema.AddMetadata(metadata, "docker_repository", &function.DockerRepository)
	schema.AddMetadata(metadata, "docker_registry", &function.DockerRegistry)

	// Environment variables count
	if function.HttpsTrigger != nil {
		schema.AddMetadata(metadata, "security_level", &function.HttpsTrigger.SecurityLevel)
	}
	if function.EventTrigger != nil {
		schema.AddMetadata(metadata, "event_type", &function.EventTrigger.EventType)
		schema.AddMetadata(metadata, "event_resource", &function.EventTrigger.Resource)
		schema.AddMetadata(metadata, "event_service", &function.EventTrigger.Service)
	}

	// Labels
	if len(function.Labels) > 0 {
		var labelPairs []string
		for k, v := range function.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
		}
		labelString := strings.Join(labelPairs, ",")
		schema.AddMetadata(metadata, "labels", &labelString)
	}

	if function.SourceRepository != nil {
		schema.AddMetadata(metadata, "source_repository_url", &function.SourceRepository.Url)
		schema.AddMetadata(metadata, "source_repository_deployed_url", &function.SourceRepository.DeployedUrl)
	}
	if function.SecretEnvironmentVariables != nil {
		schema.AddMetadataInt(metadata, "secret_env_vars_count", len(function.SecretEnvironmentVariables))
	}
	if function.SecretVolumes != nil {
		schema.AddMetadataInt(metadata, "secret_volumes_count", len(function.SecretVolumes))
	}

	return metadata
}
