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

func (d *instanceProvider) name() string {
	return "instance"
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

var ip4Regex = regexp.MustCompile(`(?:[0-9]{1,3})\.(?:[0-9]{1,3})\.(?:[0-9]{1,3})\.(?:[0-9]{1,3})`)
var ip6Regex = regexp.MustCompile(`(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))`)

func (d *instanceProvider) extractIPsFromText(text string) *schema.Resources {
	resources := schema.NewResources()

	matches := ip4Regex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		resources.Append(&schema.Resource{
			Provider:   providerName,
			ID:         d.id,
			PublicIPv4: match[0],
			Service:    d.name(),
		})
	}

	matches = ip6Regex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		resources.Append(&schema.Resource{
			Provider:   providerName,
			ID:         d.id,
			PublicIPv6: match[0],
			Service:    d.name(),
		})
	}

	return resources
}
