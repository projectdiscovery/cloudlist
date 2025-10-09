package digitalocean

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for digitalocean API
type instanceProvider struct {
	id               string
	client           *godo.Client
	extendedMetadata bool
}

func (d *instanceProvider) name() string {
	return "instance"
}

// GetInstances returns all the instances in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := schema.NewResources()

	for {
		droplets, resp, err := d.client.Droplets.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, droplet := range droplets {
			ip4, _ := droplet.PublicIPv4()
			ip6, _ := droplet.PublicIPv6()
			privateIP4, _ := droplet.PrivateIPv4()

			// Extract metadata for this droplet
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getDropletMetadata(&droplet)
			}

			if privateIP4 != "" {
				list.Append(&schema.Resource{
					Provider:    providerName,
					ID:          d.id,
					PrivateIpv4: privateIP4,
					Service:     d.name(),
					Metadata:    metadata,
				})
			}
			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				PublicIPv4: ip4,
				PublicIPv6: ip6,
				Public:     true,
				Service:    d.name(),
				Metadata:   metadata,
			})
		}
		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		opt.Page = page + 1
	}
	return list, nil
}

func (d *instanceProvider) getDropletMetadata(droplet *godo.Droplet) map[string]string {
	metadata := make(map[string]string)

	// Basic droplet information
	dropletID := fmt.Sprintf("%d", droplet.ID)
	schema.AddMetadata(metadata, "droplet_id", &dropletID)
	schema.AddMetadata(metadata, "name", &droplet.Name)
	schema.AddMetadata(metadata, "status", &droplet.Status)
	locked := fmt.Sprintf("%v", droplet.Locked)
	schema.AddMetadata(metadata, "locked", &locked)

	schema.AddMetadataInt(metadata, "memory", droplet.Memory)
	schema.AddMetadataInt(metadata, "vcpus", droplet.Vcpus)
	schema.AddMetadataInt(metadata, "disk", droplet.Disk)
	schema.AddMetadata(metadata, "size_slug", &droplet.SizeSlug)

	if droplet.Region != nil {
		schema.AddMetadata(metadata, "region_name", &droplet.Region.Name)
		schema.AddMetadata(metadata, "region_slug", &droplet.Region.Slug)
	}
	if droplet.Image != nil {
		imageID := fmt.Sprintf("%d", droplet.Image.ID)
		schema.AddMetadata(metadata, "image_id", &imageID)
		schema.AddMetadata(metadata, "image_name", &droplet.Image.Name)
		if droplet.Image.Slug != "" {
			schema.AddMetadata(metadata, "image_slug", &droplet.Image.Slug)
		}
	}
	if droplet.Size != nil {
		schema.AddMetadataInt(metadata, "size_memory", droplet.Size.Memory)
		schema.AddMetadataInt(metadata, "size_vcpus", droplet.Size.Vcpus)
		schema.AddMetadataInt(metadata, "size_disk", droplet.Size.Disk)
		schema.AddMetadata(metadata, "droplet_size_slug", &droplet.Size.Slug)
	}
	if droplet.Created != "" {
		if createdTime, err := time.Parse(time.RFC3339, droplet.Created); err == nil {
			createdAt := createdTime.Format(time.RFC3339)
			schema.AddMetadata(metadata, "created_at", &createdAt)
		} else {
			schema.AddMetadata(metadata, "created_at", &droplet.Created)
		}
	}
	if len(droplet.Features) > 0 {
		features := strings.Join(droplet.Features, ",")
		schema.AddMetadata(metadata, "features", &features)
	}
	if len(droplet.Tags) > 0 {
		tags := strings.Join(droplet.Tags, ",")
		schema.AddMetadata(metadata, "tags", &tags)
	}
	if len(droplet.BackupIDs) > 0 {
		var backupIDs []string
		for _, id := range droplet.BackupIDs {
			backupIDs = append(backupIDs, fmt.Sprintf("%d", id))
		}
		backupIDsStr := strings.Join(backupIDs, ",")
		schema.AddMetadata(metadata, "backup_ids", &backupIDsStr)
	}
	if droplet.NextBackupWindow != nil {
		nextBackupStart := droplet.NextBackupWindow.Start.Format(time.RFC3339)
		nextBackupEnd := droplet.NextBackupWindow.End.Format(time.RFC3339)
		schema.AddMetadata(metadata, "next_backup_start", &nextBackupStart)
		schema.AddMetadata(metadata, "next_backup_end", &nextBackupEnd)
	}
	if len(droplet.SnapshotIDs) > 0 {
		var snapshotIDs []string
		for _, id := range droplet.SnapshotIDs {
			snapshotIDs = append(snapshotIDs, fmt.Sprintf("%d", id))
		}
		snapshotIDsStr := strings.Join(snapshotIDs, ",")
		schema.AddMetadata(metadata, "snapshot_ids", &snapshotIDsStr)
	}

	if len(droplet.VolumeIDs) > 0 {
		volumeIDs := strings.Join(droplet.VolumeIDs, ",")
		schema.AddMetadata(metadata, "volume_ids", &volumeIDs)
		schema.AddMetadataInt(metadata, "volume_count", len(droplet.VolumeIDs))
	}
	if droplet.VPCUUID != "" {
		schema.AddMetadata(metadata, "vpc_uuid", &droplet.VPCUUID)
	}
	if droplet.Kernel != nil {
		kernelID := fmt.Sprintf("%d", droplet.Kernel.ID)
		schema.AddMetadata(metadata, "kernel_id", &kernelID)
	}

	return metadata
}
