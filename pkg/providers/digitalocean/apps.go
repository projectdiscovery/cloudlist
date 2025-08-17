package digitalocean

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalocean/godo"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// appsProvider is an instance provider for digitalocean API
type appsProvider struct {
	id               string
	client           *godo.Client
	extendedMetadata bool
}

func (d *appsProvider) name() string {
	return "app"
}

// GetInstances returns all the instances in the store for a provider.
func (d *appsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	opt := &godo.ListOptions{PerPage: 200}
	list := schema.NewResources()

	for {
		apps, resp, err := d.client.Apps.List(ctx, opt)
		if err != nil {
			return nil, err
		}

		for _, app := range apps {
			dnsname := app.LiveDomain

			// Extract metadata for this app
			var metadata map[string]string
			if d.extendedMetadata {
				metadata = d.getAppMetadata(app)
			}

			list.Append(&schema.Resource{
				Provider: providerName,
				ID:       d.id,
				DNSName:  dnsname,
				Public:   true,
				Service:  d.name(),
				Metadata: metadata,
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

func (d *appsProvider) getAppMetadata(app *godo.App) map[string]string {
	metadata := make(map[string]string)

	// Basic app information
	schema.AddMetadata(metadata, "app_id", &app.ID)
	schema.AddMetadata(metadata, "owner_uuid", &app.OwnerUUID)
	schema.AddMetadata(metadata, "live_url", &app.LiveURL)
	schema.AddMetadata(metadata, "live_domain", &app.LiveDomain)
	schema.AddMetadata(metadata, "region_slug", &app.Region.Slug)
	schema.AddMetadata(metadata, "tier_slug", &app.TierSlug)
	schema.AddMetadata(metadata, "project_id", &app.ProjectID)

	if !app.CreatedAt.IsZero() {
		createdAt := app.CreatedAt.Format(time.RFC3339)
		schema.AddMetadata(metadata, "created_at", &createdAt)
	}
	if !app.UpdatedAt.IsZero() {
		updatedAt := app.UpdatedAt.Format(time.RFC3339)
		schema.AddMetadata(metadata, "updated_at", &updatedAt)
	}
	if app.ActiveDeployment != nil {
		schema.AddMetadata(metadata, "active_deployment_id", &app.ActiveDeployment.ID)
		if app.ActiveDeployment.Phase != "" {
			phase := string(app.ActiveDeployment.Phase)
			schema.AddMetadata(metadata, "active_deployment_phase", &phase)
		}
	}
	if !app.LastDeploymentActiveAt.IsZero() {
		lastDeploymentActiveAt := app.LastDeploymentActiveAt.Format(time.RFC3339)
		schema.AddMetadata(metadata, "last_deployment_active_at", &lastDeploymentActiveAt)
	}
	if len(app.Domains) > 0 {
		domainCount := fmt.Sprintf("%d", len(app.Domains))
		schema.AddMetadata(metadata, "domain_count", &domainCount)
	}

	return metadata
}
