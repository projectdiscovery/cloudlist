package inventory

import (
	"os"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	"gopkg.in/yaml.v2"
)

// ParseOptions parses the options returning option structure
func ParseOptions(path string) (schema.Options, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			gologger.Error().Msgf("Could not close provider config file: %s\n", err)
		}
	}()

	options := schema.Options{}
	if err := yaml.NewDecoder(file).Decode(&options); err != nil {
		return nil, err
	}
	return options, nil
}
