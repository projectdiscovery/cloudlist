package aws

import (
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

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
