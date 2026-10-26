package runner

import (
	"path/filepath"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestSanitizePrivateIPs(t *testing.T) {
	resource := &schema.Resource{
		PublicIPv4:  "1.2.3.4",
		PrivateIpv4: "10.0.0.5",
		PrivateIpv6: "fd00::1",
		PublicIPv6:  "2606:4700:4700::1111",
		DNSName:     "example.internal",
	}

	sanitizePrivateIPs(resource, true)

	if resource.PrivateIpv4 != "" || resource.PrivateIpv6 != "" {
		t.Fatalf("expected private addresses to be cleared, got %q and %q", resource.PrivateIpv4, resource.PrivateIpv6)
	}

	if resource.PublicIPv4 == "" || resource.PublicIPv6 == "" || resource.DNSName == "" {
		t.Fatalf("public fields should remain untouched: %+v", resource)
	}

	resource.PrivateIpv4 = "10.0.0.5"
	resource.PrivateIpv6 = "fd00::1"
	sanitizePrivateIPs(resource, false)

	if resource.PrivateIpv4 == "" || resource.PrivateIpv6 == "" {
		t.Fatalf("expected private addresses to remain when exclusion disabled")
	}

	sanitizePrivateIPs(nil, true)
}

func TestSanitizePrivateIPsOnMisclassifiedFields(t *testing.T) {
	resource := &schema.Resource{
		PublicIPv4: "10.10.0.5",
		PublicIPv6: "fd00::5",
	}

	sanitizePrivateIPs(resource, true)

	if resource.PublicIPv4 != "" || resource.PublicIPv6 != "" {
		t.Fatalf("expected private addresses in public fields to be cleared, got ipv4=%q ipv6=%q", resource.PublicIPv4, resource.PublicIPv6)
	}

	resource.PublicIPv4 = "8.8.8.8"
	resource.PublicIPv6 = "2606:4700:4700::1111"
	sanitizePrivateIPs(resource, true)

	if resource.PublicIPv4 == "" || resource.PublicIPv6 == "" {
		t.Fatalf("expected public addresses to remain when exclusion enabled")
	}
}

func TestNewWithAutoDiscoveryIgnoresProviderConfig(t *testing.T) {
	r, err := New(&Options{AutoDiscovery: true, ProviderConfig: filepath.Join(t.TempDir(), "missing.yaml")})
	require.NoError(t, err)

	var providers []string
	for _, block := range r.config {
		providers = append(providers, block["provider"])
	}
	require.Equal(t, autoDiscoveryProviders, providers)
}
