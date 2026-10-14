package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/inventory"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// autoDiscoveryProviders resolve credentials without config through their SDK
// default credential chains.
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
	if len(config) == 0 {
		return nil, fmt.Errorf("no auto-discovery provider matches the -p/-id filters, supported: %s", strings.Join(autoDiscoveryProviders, ","))
	}
	// Init fails for missing credentials but also for denied, expired or
	// unreachable ones, so skip failed providers and report why.
	discovered := &inventory.Inventory{}
	var errs []error
	for _, block := range config {
		inv, err := inventory.New(schema.Options{block})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		discovered.Providers = append(discovered.Providers, inv.Providers...)
	}
	if len(discovered.Providers) == 0 {
		return nil, fmt.Errorf("no provider could be initialized: %w", errors.Join(errs...))
	}
	for _, err := range errs {
		// Info stays visible at the default log level. Warning is dropped unless -v is set.
		gologger.Info().Msgf("Skipping provider: %s", err)
	}
	return discovered, nil
}
