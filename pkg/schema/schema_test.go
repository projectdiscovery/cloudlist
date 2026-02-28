package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestOptionBlockParsesProjectIDs(t *testing.T) {
	data := `
- provider: gcp
  project_ids:
    - alpha
    - beta
    - alpha
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("project_ids")
	require.True(t, ok)
	require.Equal(t, "alpha,beta,alpha", value)
}
