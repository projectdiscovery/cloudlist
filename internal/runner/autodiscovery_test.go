package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

func writeKubeconfig(t *testing.T) string {
	t.Helper()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://k8s.example.test:6443
contexts:
- name: test
  context:
    cluster: test
current-context: test
`), 0o600))
	return kubeconfig
}

func TestNewInventorySkipsProvidersWithoutCredentials(t *testing.T) {
	kubeconfig := writeKubeconfig(t)
	r := &Runner{options: &Options{AutoDiscovery: true}}

	inv, err := r.newInventory(schema.Options{
		{"provider": "kubernetes", "kubeconfig_file": filepath.Join(t.TempDir(), "missing")},
		{"provider": "kubernetes", "kubeconfig_file": kubeconfig},
	})
	require.NoError(t, err)
	require.Len(t, inv.Providers, 1)

	_, err = r.newInventory(schema.Options{
		{"provider": "kubernetes", "kubeconfig_file": filepath.Join(t.TempDir(), "missing")},
	})
	require.ErrorContains(t, err, "no provider could be initialized")
	require.ErrorContains(t, err, "could not create provider kubernetes")
}

func TestNewInventoryWithoutMatchingProviders(t *testing.T) {
	r := &Runner{options: &Options{AutoDiscovery: true}}

	_, err := r.newInventory(schema.Options{})
	require.ErrorContains(t, err, "no auto-discovery provider matches the -p/-id filters")
}

func TestNewInventoryFailsFastWithoutAutoDiscovery(t *testing.T) {
	r := &Runner{options: &Options{}}

	_, err := r.newInventory(schema.Options{
		{"provider": "kubernetes", "kubeconfig_file": filepath.Join(t.TempDir(), "missing")},
		{"provider": "kubernetes", "kubeconfig_file": writeKubeconfig(t)},
	})
	require.ErrorContains(t, err, "could not create provider kubernetes")
}
