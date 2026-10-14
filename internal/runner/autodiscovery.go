package runner

import (
	"errors"

	"github.com/projectdiscovery/cloudlist/pkg/inventory"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// autoDiscoveryProviders resolve credentials without config through their SDK
// default chains: environment variables, CLI logins, instance metadata and
// in-cluster service accounts.
var autoDiscoveryProviders = []string{"aws", "gcp", "azure", "kubernetes"}

func autoDiscoveryConfig() schema.Options {
	config := make(schema.Options, 0, len(autoDiscoveryProviders))
	for _, provider := range autoDiscoveryProviders {
		config = append(config, schema.OptionBlock{"provider": provider})
	}
	return config
}

func (r *Runner) newInventory(config schema.Options) (*inventory.Inventory, error) {
	if !r.options.AutoDiscovery {
		return inventory.New(config)
	}
	// A provider fails to initialize when the environment has no credentials
	// for it, which is expected here, so skip it instead of aborting.
	discovered := &inventory.Inventory{}
	for _, block := range config {
		inv, err := inventory.New(schema.Options{block})
		if err != nil {
			gologger.Verbose().Msgf("Skipping %s: %s", block["provider"], err)
			continue
		}
		discovered.Providers = append(discovered.Providers, inv.Providers...)
	}
	if len(discovered.Providers) == 0 {
		return nil, errors.New("no provider credentials found in the environment")
	}
	return discovered, nil
}
