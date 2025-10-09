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
		{"17.5.7.8", PublicIPv4},
		{"1.1.1.1", PublicIPv4},
		{"192.168.1.10", PrivateIPv4},
		{"185.199.110.153", PublicIPv4},
		{"www.example.com:22", DNSName},
		{"17.5.7.8:443", PublicIPv4},
		{"192.168.1.10:80", PrivateIPv4},
		{"2a0c:5a80::1", PublicIPv6},
		{"[2a0c:5a80::1]:80", PublicIPv6},
		{"2001:db8::1", PrivateIPv6},
		{"[2001:db8::a]:443", PrivateIPv6},
	}

	validator, err := NewValidator()
	require.Nil(t, err, "could not create validator")

	for _, test := range tests {
		resource := validator.Identify(test.item)
		require.Equal(t, test.resource, resource, "could not get correct resource for %s", test.item)
	}
}
