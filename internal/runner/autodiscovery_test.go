package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestNewInventorySkipsProvidersWithoutCredentials(t *testing.T) {
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
	require.EqualError(t, err, "no provider credentials found in the environment")
}

func TestNewInventoryFailsFastWithoutAutoDiscovery(t *testing.T) {
	r := &Runner{options: &Options{}}

	_, err := r.newInventory(schema.Options{
		{"provider": "kubernetes", "kubeconfig_file": filepath.Join(t.TempDir(), "missing")},
	})
	require.Error(t, err)
}

func TestAutoDiscoveryKubernetesUsesDefaultKubeconfig(t *testing.T) {
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
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", kubeconfig)

	var kubernetes schema.Options
	for _, block := range autoDiscoveryConfig() {
		if block["provider"] == "kubernetes" {
			kubernetes = append(kubernetes, block)
		}
	}
	r := &Runner{options: &Options{AutoDiscovery: true}}
	inv, err := r.newInventory(kubernetes)
	require.NoError(t, err)
	require.Len(t, inv.Providers, 1)
}
