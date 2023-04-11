package terraform

import (
	"context"
	"io"
	"os"
	"regexp"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// instanceProvider is an instance provider for terraform state file
type instanceProvider struct {
	path string
	id   string
}

// GetInstances returns all the instances in the store for a provider.
func (d *instanceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	file, err := os.Open(d.path)
	if err != nil {
		return nil, errors.Wrap(err, "could not open state file")
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, errors.Wrap(err, "could not read state file")
	}

	resources := d.extractIPsFromText(string(data))
	return resources, nil
}

var ipRegex = regexp.MustCompile(`(?:[0-9]{1,3})\.(?:[0-9]{1,3})\.(?:[0-9]{1,3})\.(?:[0-9]{1,3})`)

func (d *instanceProvider) extractIPsFromText(text string) *schema.Resources {
	resources := schema.NewResources()

	matches := ipRegex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		resources.Append(&schema.Resource{
			Provider:   providerName,
			ID:         d.id,
			PublicIPv4: match[0],
		})
	}
	return resources
}
