package inventory

import (
	"os"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"gopkg.in/yaml.v2"
)

// ParseOptions parses the options returning option structure
func ParseOptions(path string) (schema.Options, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	options := schema.Options{}
	if err := yaml.NewDecoder(file).Decode(&options); err != nil {
		return nil, err
	}
	return options, nil
}
