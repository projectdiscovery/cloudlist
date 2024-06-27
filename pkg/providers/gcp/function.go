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

func (d *cloudFunctionsProvider) name() string {
	return "cloud-function"
}

// GetResource returns all the Cloud Function resources in the store for a provider.
func (d *cloudFunctionsProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	functions, err := d.getFunctions()
	if err != nil {
		return nil, fmt.Errorf("could not get functions: %s", err)
	}
	for _, function := range functions {
		funcUrl, _ := url.Parse(function.HttpsTrigger.Url)
		resource := &schema.Resource{
			ID:       d.id,
			Provider: providerName,
			DNSName:  funcUrl.Hostname(),
			Public:   d.isPublicFunction(function.Name),
			Service:  d.name(),
		}
		list.Append(resource)
	}
	return list, nil
}

func (d *cloudFunctionsProvider) getFunctions() ([]*cloudfunctions.CloudFunction, error) {
	var functions []*cloudfunctions.CloudFunction
	for _, project := range d.projects {
		functionsService := d.functions.Projects.Locations.Functions.List(fmt.Sprintf("projects/%s/locations/-", project))
		_ = functionsService.Pages(context.Background(), func(fal *cloudfunctions.ListFunctionsResponse) error {
			functions = append(functions, fal.Functions...)
			return nil
		})
	}
	return functions, nil
}

func (d *cloudFunctionsProvider) isPublicFunction(functionName string) bool {
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
