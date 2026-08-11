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

func TestOptionBlockParsesExcludeServices(t *testing.T) {
	data := `
- provider: gcp
  exclude_services:
    - cloud-function
    - gke
`
	var options Options
	err := yaml.Unmarshal([]byte(data), &options)
	require.NoError(t, err)
	require.Len(t, options, 1)

	value, ok := options[0].GetMetadata("exclude_services")
	require.True(t, ok)
	require.Equal(t, "cloud-function,gke", value)
	require.Equal(t, []string{"cloud-function", "gke"}, options.GetExcludeServiceNames())
}

func TestResolveServices(t *testing.T) {
	supported := []string{"ec2", "s3", "lambda", "route53"}

	tests := []struct {
		name        string
		block       OptionBlock
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "no options defaults to all supported",
			block:       OptionBlock{},
			wantPresent: supported,
		},
		{
			name:        "allowlist keeps only listed services",
			block:       OptionBlock{"services": "ec2,s3"},
			wantPresent: []string{"ec2", "s3"},
			wantAbsent:  []string{"lambda", "route53"},
		},
		{
			name:        "exclude drops from default-all set",
			block:       OptionBlock{"exclude_services": "s3,route53"},
			wantPresent: []string{"ec2", "lambda"},
			wantAbsent:  []string{"s3", "route53"},
		},
		{
			name:        "exclude composes with allowlist",
			block:       OptionBlock{"services": "ec2,s3,lambda", "exclude_services": "s3"},
			wantPresent: []string{"ec2", "lambda"},
			wantAbsent:  []string{"s3"},
		},
		{
			name:        "whitespace is trimmed in both lists",
			block:       OptionBlock{"services": "ec2, s3, lambda", "exclude_services": "s3, lambda"},
			wantPresent: []string{"ec2"},
			wantAbsent:  []string{"s3", "lambda"},
		},
		{
			name:        "unknown allowlist values are ignored and fall back to all",
			block:       OptionBlock{"services": "not-a-service"},
			wantPresent: supported,
		},
		{
			name:        "unknown exclude values are ignored",
			block:       OptionBlock{"exclude_services": "not-a-service"},
			wantPresent: supported,
		},
		{
			name:       "excluding every service yields an empty set",
			block:      OptionBlock{"exclude_services": "ec2,s3,lambda,route53"},
			wantAbsent: supported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services := tt.block.ResolveServices(supported)
			for _, s := range tt.wantPresent {
				require.Truef(t, services.Has(s), "expected service %q to be present", s)
			}
			for _, s := range tt.wantAbsent {
				require.Falsef(t, services.Has(s), "expected service %q to be absent", s)
			}
		})
	}
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
