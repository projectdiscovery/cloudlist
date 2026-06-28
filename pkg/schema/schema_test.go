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

func TestOptionBlockParsesAccountIDs(t *testing.T) {
	data := `
- provider: aws
  account_ids:
    - "123456789012"
    - "234567890123"
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("account_ids")
	require.True(t, ok)
	require.Equal(t, "123456789012,234567890123", value)
}

func TestOptionBlockZeroPadsUnquotedAccountIDs(t *testing.T) {
	data := `
- provider: aws
  account_ids:
    - 012345678901
    - 123456789012
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("account_ids")
	require.True(t, ok)
	require.Equal(t, "012345678901,123456789012", value)
}

func TestOptionBlockZeroPadsExcludeAccountIDs(t *testing.T) {
	data := `
- provider: aws
  exclude_account_ids:
    - 012345678901
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("exclude_account_ids")
	require.True(t, ok)
	require.Equal(t, "012345678901", value)
}

func TestOptionBlockScalarFallback(t *testing.T) {
	data := `
- provider: aws
  assume_role_name: PDScannerRole
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("assume_role_name")
	require.True(t, ok)
	require.Equal(t, "PDScannerRole", value)
}

func TestOptionBlockParsesExcludeServices(t *testing.T) {
	data := `
- provider: gcp
  exclude_services:
    - cloud-function
    - cloud-run
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("exclude_services")
	require.True(t, ok)
	require.Equal(t, "cloud-function,cloud-run", value)
	require.Equal(t, []string{"cloud-function", "cloud-run"}, options.GetExcludeServiceNames())

	supported := []string{"dns", "compute", "gke", "cloud-function", "cloud-run"}
	serviceMap := options[0].ParseServices(supported)
	require.True(t, serviceMap.Has("dns"))
	require.True(t, serviceMap.Has("compute"))
	require.True(t, serviceMap.Has("gke"))
	require.False(t, serviceMap.Has("cloud-function"))
	require.False(t, serviceMap.Has("cloud-run"))
}
