package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateIdentify(t *testing.T) {
	tests := []struct {
		item     string
		resource ResourceType
	}{
		{"www.example.com", DNSName},
		{"17.5.7.8", PublicIP},
		{"1.1.1.1", PublicIP},
		{"192.168.1.10", PrivateIP},
		{"185.199.110.153", PublicIP},
	}

	validator, err := NewValidator()
	require.Nil(t, err, "could not create validator")

	for _, test := range tests {
		resource := validator.Identify(test.item)
		require.Equal(t, test.resource, resource, "could not get correct resource for %s", test.item)
	}
}
