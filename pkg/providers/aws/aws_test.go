package aws

import (
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

// TestParseOptionBlockExcludeServices covers exclude_services resolution:
// the excluded entries are dropped from the effective set, exclusion composes
// with the services allowlist, and unknown values are ignored.
func TestParseOptionBlockExcludeServices(t *testing.T) {
	tests := []struct {
		name        string
		block       schema.OptionBlock
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "exclude from default-all set",
			block:       schema.OptionBlock{"exclude_services": "s3,route53"},
			wantPresent: []string{"ec2", "lambda"},
			wantAbsent:  []string{"s3", "route53"},
		},
		{
			name:        "exclude composes with services allowlist",
			block:       schema.OptionBlock{"services": "ec2,s3,lambda", "exclude_services": "s3"},
			wantPresent: []string{"ec2", "lambda"},
			wantAbsent:  []string{"s3"},
		},
		{
			name:        "whitespace is trimmed in both lists",
			block:       schema.OptionBlock{"services": "ec2, s3, lambda", "exclude_services": "s3, lambda"},
			wantPresent: []string{"ec2"},
			wantAbsent:  []string{"s3", "lambda"},
		},
		{
			name:        "unknown exclude value is ignored",
			block:       schema.OptionBlock{"exclude_services": "not-a-service"},
			wantPresent: []string{"ec2", "s3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts ProviderOptions
			require.NoError(t, opts.ParseOptionBlock(tt.block))
			for _, s := range tt.wantPresent {
				require.True(t, opts.Services.Has(s), "expected service %q to be present", s)
			}
			for _, s := range tt.wantAbsent {
				require.False(t, opts.Services.Has(s), "expected service %q to be excluded", s)
			}
		})
	}
}

// TestParseOptionBlock covers the static-credential pair validation that backs
// keyless authentication: both keys present is valid, both omitted is valid
// (keyless), and a half-configured pair is rejected.
func TestParseOptionBlock(t *testing.T) {
	tests := []struct {
		name          string
		block         schema.OptionBlock
		wantErr       bool
		wantAccessKey string
		wantSecretKey string
	}{
		{
			name:          "both keys present",
			block:         schema.OptionBlock{"aws_access_key": "AKIAEXAMPLE", "aws_secret_key": "secret"},
			wantAccessKey: "AKIAEXAMPLE",
			wantSecretKey: "secret",
		},
		{
			name:  "both keys omitted is keyless",
			block: schema.OptionBlock{},
		},
		{
			name:    "only access key is an error",
			block:   schema.OptionBlock{"aws_access_key": "AKIAEXAMPLE"},
			wantErr: true,
		},
		{
			name:    "only secret key is an error",
			block:   schema.OptionBlock{"aws_secret_key": "secret"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts ProviderOptions
			err := opts.ParseOptionBlock(tt.block)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantAccessKey, opts.AccessKey)
			require.Equal(t, tt.wantSecretKey, opts.SecretKey)
		})
	}
}
