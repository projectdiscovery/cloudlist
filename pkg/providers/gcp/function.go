package gcp

import (
	"context"
	"fmt"
	"net/url"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"google.golang.org/api/cloudfunctions/v1"
)

type cloudFunctionsProvider struct {
	id        string
	functions *cloudfunctions.Service
	projects  []string
}

// GetResource returns all the Cloud Function resources in the store for a provider.
func (d *cloudFunctionsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, project := range d.projects {
		functionsService := d.functions.Projects.Locations.Functions.List(fmt.Sprintf("projects/%s/locations/-", project))
		err := functionsService.Pages(context.Background(), func(fal *cloudfunctions.ListFunctionsResponse) error {
			for _, function := range fal.Functions {
				funcUrl, _ := url.Parse(function.HttpsTrigger.Url)
				resource := &schema.Resource{
					ID:       d.id,
					Provider: providerName,
					DNSName:  funcUrl.Hostname(),
				}
				functionIAMPolicy, err := d.functions.Projects.Locations.Functions.GetIamPolicy(function.Name).Do()
				if err == nil {
					for _, binding := range functionIAMPolicy.Bindings {
						if binding.Role == "roles/cloudfunctions.invoker" {
							for _, member := range binding.Members {
								if member == "allUsers" || member == "allAuthenticatedUsers" {
									resource.Public = true
									break
								}
							}
						}
					}
				}
				list.Append(resource)
			}
			return nil
		})
		if err != nil {
			continue
		}
	}
	return list, nil
}
