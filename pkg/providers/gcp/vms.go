package gcp

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/compute/v1"
)

type cloudVMProvider struct {
	id               string
	compute          *compute.Service
	projects         []string
	extendedMetadata bool
}

func (d *cloudVMProvider) name() string {
	return "vms"
}

// GetResource returns all the resources in the store for a provider.
func (d *cloudVMProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		instances := d.compute.Instances.AggregatedList(project)
		err := instances.Pages(context.Background(), func(ial *compute.InstanceAggregatedList) error {
			for zone, instancesScopedList := range ial.Items {
				for _, instance := range instancesScopedList.Instances {
					instance := instance

					if len(instance.NetworkInterfaces) == 0 {
						continue
					}
					nic := instance.NetworkInterfaces[0]
					if len(nic.AccessConfigs) == 0 {
						continue
					}
					cfg := nic.AccessConfigs[0]

					var metadata map[string]string
					if d.extendedMetadata {
						metadata = d.getInstanceMetadata(instance, project, zone)
					}

					list.Append(&schema.Resource{
						ID:         d.id,
						Public:     true,
						Provider:   providerName,
						PublicIPv4: cfg.NatIP,
						PublicIPv6: cfg.ExternalIpv6,
						Service:    d.name(),
						Metadata:   metadata,
					})
				}
			}
			return nil
		})
		if err != nil {
			log.Printf("Could not get all instances for project %s: %s\n", project, err)
			continue
		}
	}
	return list, nil
}

func (d *cloudVMProvider) getInstanceMetadata(instance *compute.Instance, project, zone string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "instance_name", &instance.Name)
	if instance.Id != 0 {
		metadata["instance_id"] = fmt.Sprintf("%d", instance.Id)
	}
	machineType := extractResourceName(instance.MachineType)
	schema.AddMetadata(metadata, "machine_type", &machineType)
	schema.AddMetadata(metadata, "status", &instance.Status)
	zoneExtracted := extractResourceName(instance.Zone)
	schema.AddMetadata(metadata, "zone", &zoneExtracted)

	metadata["project_id"] = project
	metadata["owner_id"] = project // In GCP, project ID serves as owner identifier

	schema.AddMetadata(metadata, "creation_timestamp", &instance.CreationTimestamp)
	schema.AddMetadata(metadata, "last_start_timestamp", &instance.LastStartTimestamp)
	schema.AddMetadata(metadata, "last_stop_timestamp", &instance.LastStopTimestamp)
	schema.AddMetadata(metadata, "last_suspended_timestamp", &instance.LastSuspendedTimestamp)

	schema.AddMetadata(metadata, "cpu_platform", &instance.CpuPlatform)
	schema.AddMetadata(metadata, "min_cpu_platform", &instance.MinCpuPlatform)
	schema.AddMetadata(metadata, "hostname", &instance.Hostname)
	schema.AddMetadata(metadata, "description", &instance.Description)

	if instance.DeletionProtection {
		metadata["deletion_protection"] = "true"
	}
	if instance.CanIpForward {
		metadata["can_ip_forward"] = "true"
	}
	if instance.StartRestricted {
		metadata["start_restricted"] = "true"
	}

	if len(instance.Labels) > 0 {
		var labelPairs []string
		for key, value := range instance.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", key, value))
		}
		metadata["labels"] = strings.Join(labelPairs, ",")
	}

	if instance.Tags != nil && len(instance.Tags.Items) > 0 {
		metadata["firewall_tags"] = strings.Join(instance.Tags.Items, ",")
		schema.AddMetadata(metadata, "tags_fingerprint", &instance.Tags.Fingerprint)
	}

	if len(instance.ServiceAccounts) > 0 {
		var emails []string
		for _, sa := range instance.ServiceAccounts {
			if sa.Email != "" {
				emails = append(emails, sa.Email)
			}
		}
		if len(emails) > 0 {
			metadata["service_accounts"] = strings.Join(emails, ",")
		}
	}

	if len(instance.NetworkInterfaces) > 0 {
		nic := instance.NetworkInterfaces[0]
		network := extractResourceName(nic.Network)
		schema.AddMetadata(metadata, "network", &network)
		subnetwork := extractResourceName(nic.Subnetwork)
		schema.AddMetadata(metadata, "subnetwork", &subnetwork)
		schema.AddMetadata(metadata, "network_ip", &nic.NetworkIP)

		if len(instance.NetworkInterfaces) > 1 {
			metadata["network_interfaces_count"] = fmt.Sprintf("%d", len(instance.NetworkInterfaces))
		}
	}

	if len(instance.Disks) > 0 {
		metadata["disks_count"] = fmt.Sprintf("%d", len(instance.Disks))

		for _, disk := range instance.Disks {
			if disk.Boot {
				schema.AddMetadata(metadata, "boot_disk_name", &disk.DeviceName)
				diskSource := extractResourceName(disk.Source)
				schema.AddMetadata(metadata, "boot_disk_source", &diskSource)
				if disk.DiskSizeGb != 0 {
					metadata["boot_disk_size_gb"] = fmt.Sprintf("%d", disk.DiskSizeGb)
				}
				break
			}
		}
	}

	if instance.Scheduling != nil {
		if instance.Scheduling.Preemptible {
			metadata["preemptible"] = "true"
		}
		schema.AddMetadata(metadata, "on_host_maintenance", &instance.Scheduling.OnHostMaintenance)
		if instance.Scheduling.AutomaticRestart != nil {
			metadata["automatic_restart"] = fmt.Sprintf("%v", *instance.Scheduling.AutomaticRestart)
		}
	}

	if instance.Metadata != nil && len(instance.Metadata.Items) > 0 {
		sensitiveKeys := map[string]bool{
			"ssh-keys":           true,
			"startup-script":     true,
			"startup-script-url": true,
			"shutdown-script":    true,
			"user-data":          true,
		}
		for _, item := range instance.Metadata.Items {
			if item.Key != "" && item.Value != nil {
				_, sensitive := sensitiveKeys[strings.ToLower(item.Key)]
				if !sensitive {
					metadata[fmt.Sprintf("user_metadata_%s", strings.ToLower(item.Key))] = *item.Value
				}
			}
		}
	}
	return metadata
}

// extractResourceName extracts the resource name from a full GCP resource URL
func extractResourceName(resourceURL string) string {
	if resourceURL == "" {
		return ""
	}
	parts := strings.Split(resourceURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return resourceURL
}
