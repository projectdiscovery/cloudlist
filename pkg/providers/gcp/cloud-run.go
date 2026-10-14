package gcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	run "google.golang.org/api/run/v1"
)

type cloudRunProvider struct {
	id               string
	run              *run.APIService
	projects         []string
	extendedMetadata bool
}

func (d *cloudRunProvider) name() string {
	return "cloud-run"
}

// GetResource returns all the Cloud Run resources in the store for a provider.
func (d *cloudRunProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	services, err := d.getServices()
	if err != nil {
		return nil, fmt.Errorf("could not get services: %s", err)
	}
	for _, service := range services {
		serviceUrl, _ := url.Parse(service.Status.Url)

		var metadata map[string]string
		if d.extendedMetadata {
			metadata = d.getServiceMetadata(service)
		}

		resource := &schema.Resource{
			ID:       d.id,
			Provider: providerName,
			DNSName:  serviceUrl.Hostname(),
			Public:   d.isPublicService(service.Metadata.Name),
			Service:  d.name(),
			Metadata: metadata,
		}
		list.Append(resource)
	}
	return list, nil
}

func (d *cloudRunProvider) getServices() ([]*run.Service, error) {
	var services []*run.Service
	for _, project := range d.projects {
		locationsService := d.run.Projects.Locations.List(fmt.Sprintf("projects/%s", project))
		locationsResponse, err := locationsService.Do()
		if err != nil {
			gologger.Warning().Msgf("could not list cloud run locations for project %s: %s", project, err)
			continue
		}

		for _, location := range locationsResponse.Locations {
			// build the parent explicitly because the location.Name is not returned with the projects/ prefix in Knative API
			parent := fmt.Sprintf("projects/%s/locations/%s", project, location.LocationId)
			servicesResponse, err := d.run.Projects.Locations.Services.List(parent).Do()
			if err != nil {
				gologger.Warning().Msgf("could not list cloud run services for %s: %s", parent, err)
				continue
			}
			services = append(services, servicesResponse.Items...)
		}
	}
	return services, nil
}

func (d *cloudRunProvider) isPublicService(serviceName string) bool {
	serviceIAMPolicy, err := d.run.Projects.Locations.Services.GetIamPolicy(serviceName).Do()
	if err == nil {
		for _, binding := range serviceIAMPolicy.Bindings {
			if binding.Role == "roles/run.invoker" {
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

func (d *cloudRunProvider) getServiceMetadata(service *run.Service) map[string]string {
	metadata := make(map[string]string)

	// Basic service information
	if service.Metadata != nil {
		schema.AddMetadata(metadata, "service_name", &service.Metadata.Name)
		schema.AddMetadata(metadata, "namespace", &service.Metadata.Namespace)
		schema.AddMetadata(metadata, "uid", &service.Metadata.Uid)
		schema.AddMetadata(metadata, "resource_version", &service.Metadata.ResourceVersion)
		schema.AddMetadata(metadata, "self_link", &service.Metadata.SelfLink)
		schema.AddMetadata(metadata, "creation_timestamp", &service.Metadata.CreationTimestamp)

		if service.Metadata.Generation != 0 {
			metadata["generation"] = fmt.Sprintf("%d", service.Metadata.Generation)
		}

		if service.Metadata.Namespace != "" {
			parts := strings.Split(service.Metadata.Namespace, "/")
			if len(parts) >= 2 && parts[0] == "projects" {
				metadata["project_id"] = parts[1]
				metadata["owner_id"] = parts[1]
			}
			if len(parts) >= 4 && parts[2] == "locations" {
				metadata["location"] = parts[3]
			}
		}

		// Labels
		if len(service.Metadata.Labels) > 0 {
			var labelPairs []string
			for key, value := range service.Metadata.Labels {
				labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", key, value))
			}
			metadata["labels"] = strings.Join(labelPairs, ",")
		}

		// Annotations
		if len(service.Metadata.Annotations) > 0 {
			// Extract important annotations
			if val, ok := service.Metadata.Annotations["run.googleapis.com/ingress"]; ok {
				metadata["ingress"] = val
			}
			if val, ok := service.Metadata.Annotations["run.googleapis.com/launch-stage"]; ok {
				metadata["launch_stage"] = val
			}
			if val, ok := service.Metadata.Annotations["serving.knative.dev/creator"]; ok {
				metadata["creator"] = val
			}
			if val, ok := service.Metadata.Annotations["serving.knative.dev/lastModifier"]; ok {
				metadata["last_modifier"] = val
			}
			if val, ok := service.Metadata.Annotations["client.knative.dev/user-image"]; ok {
				metadata["user_image"] = val
			}
		}
	}

	if service.Status != nil {
		schema.AddMetadata(metadata, "url", &service.Status.Url)
		if service.Status.ObservedGeneration != 0 {
			metadata["observed_generation"] = fmt.Sprintf("%d", service.Status.ObservedGeneration)
		}

		schema.AddMetadata(metadata, "latest_ready_revision", &service.Status.LatestReadyRevisionName)
		schema.AddMetadata(metadata, "latest_created_revision", &service.Status.LatestCreatedRevisionName)
	}

	// Spec information
	if service.Spec != nil && service.Spec.Template != nil {
		template := service.Spec.Template

		// Container information
		if template.Spec != nil && len(template.Spec.Containers) > 0 {
			container := template.Spec.Containers[0]
			schema.AddMetadata(metadata, "container_image", &container.Image)

			if len(container.Ports) > 0 && container.Ports[0].ContainerPort != 0 {
				metadata["container_port"] = fmt.Sprintf("%d", container.Ports[0].ContainerPort)
			}
			if container.Resources != nil && container.Resources.Limits != nil {
				if cpu, ok := container.Resources.Limits["cpu"]; ok {
					metadata["cpu_limit"] = cpu
				}
				if memory, ok := container.Resources.Limits["memory"]; ok {
					metadata["memory_limit"] = memory
				}
			}
		}

		// Service account
		schema.AddMetadata(metadata, "service_account", &template.Spec.ServiceAccountName)
	}

	return metadata
}
