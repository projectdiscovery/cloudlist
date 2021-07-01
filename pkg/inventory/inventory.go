package inventory

import (
	"fmt"

	"github.com/projectdiscovery/cloudlist/pkg/providers/alibaba"
	"github.com/projectdiscovery/cloudlist/pkg/providers/aws"
	"github.com/projectdiscovery/cloudlist/pkg/providers/digitalocean"
	"github.com/projectdiscovery/cloudlist/pkg/providers/fastly"
	"github.com/projectdiscovery/cloudlist/pkg/providers/gcp"
	"github.com/projectdiscovery/cloudlist/pkg/providers/heroku"
	"github.com/projectdiscovery/cloudlist/pkg/providers/linode"
	"github.com/projectdiscovery/cloudlist/pkg/providers/scaleway"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// Inventory is an inventory of providers
type Inventory struct {
	Providers []schema.Provider
}

// New creates a new inventory of providers
func New(options schema.Options) (*Inventory, error) {
	inventory := &Inventory{}

	for _, block := range options {
		value, ok := block.GetMetadata("provider")
		if !ok {
			continue
		}
		profile, _ := block.GetMetadata("profile")
		provider, err := nameToProvider(value, block)
		if err != nil {
			gologger.Warning().Msgf("Could not initialize provider %s %s: %s\n", value, profile, err)
			continue
		}
		inventory.Providers = append(inventory.Providers, provider)
	}
	return inventory, nil
}

// nameToProvider returns the provider for a name
func nameToProvider(value string, block schema.OptionBlock) (schema.Provider, error) {
	switch value {
	case "aws":
		return aws.New(block)
	case "do":
		return digitalocean.New(block)
	case "gcp":
		return gcp.New(block)
	case "scw":
		return scaleway.New(block)
	case "heroku":
		return heroku.New(block)
	case "linode":
		return linode.New(block)
	case "fastly":
		return fastly.New(block)
	case "alibaba":
		return alibaba.New(block)
	default:
		return nil, fmt.Errorf("invalid provider name found: %s", value)
	}
}
