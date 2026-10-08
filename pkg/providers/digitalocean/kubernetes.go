package digitalocean

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// kubernetesProvider is a DOKS cluster provider for digitalocean API
type kubernetesProvider struct {
	id               string
	client           *godo.Client
	extendedMetadata bool
}

func (d *kubernetesProvider) name() string {
	return "kubernetes"
}

// GetResource returns the control plane endpoints of all DOKS clusters.
func (d *kubernetesProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := schema.NewResources()

	for {
		clusters, resp, err := d.client.Kubernetes.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, cluster := range clusters {
			if cluster == nil {
				continue
			}

			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getClusterMetadata(cluster)
			}

			// Endpoint is a URL (https://<id>.k8s.ondigitalocean.com), not a hostname.
			var hostname string
			if u, err := url.Parse(cluster.Endpoint); err == nil {
				hostname = u.Hostname()
			}

			list.Append(&schema.Resource{
				Provider:   providerName,
				ID:         d.id,
				DNSName:    hostname,
				PublicIPv4: cluster.IPv4,
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

func (d *kubernetesProvider) getClusterMetadata(cluster *godo.KubernetesCluster) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "cluster_id", &cluster.ID)
	schema.AddMetadata(metadata, "name", &cluster.Name)
	schema.AddMetadata(metadata, "region_slug", &cluster.RegionSlug)
	schema.AddMetadata(metadata, "version", &cluster.VersionSlug)
	schema.AddMetadata(metadata, "endpoint", &cluster.Endpoint)
	schema.AddMetadata(metadata, "vpc_uuid", &cluster.VPCUUID)

	if cluster.Status != nil {
		state := string(cluster.Status.State)
		schema.AddMetadata(metadata, "status", &state)
	}
	if len(cluster.Tags) > 0 {
		tags := strings.Join(cluster.Tags, ",")
		schema.AddMetadata(metadata, "tags", &tags)
	}
	schema.AddMetadataInt(metadata, "node_pool_count", len(cluster.NodePools))
	if !cluster.CreatedAt.IsZero() {
		createdAt := cluster.CreatedAt.Format(time.RFC3339)
		schema.AddMetadata(metadata, "created_at", &createdAt)
	}

	return metadata
}
