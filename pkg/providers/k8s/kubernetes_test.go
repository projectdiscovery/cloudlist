package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: one
  cluster:
    server: https://one.example.test:6443
- name: two
  cluster:
    server: https://two.example.test:6443
users:
- name: test
  user:
    token: test
contexts:
- name: one
  context:
    cluster: one
    user: test
- name: two
  context:
    cluster: two
    user: test
current-context: %s
`

func writeKubeconfig(t *testing.T, currentContext string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(testKubeconfig, currentContext)), 0o600))
	return path
}

func host(t *testing.T, p *Provider) string {
	t.Helper()
	return p.clientSet.CoreV1().RESTClient().Get().URL().Host
}

func TestNew(t *testing.T) {
	envConfig := writeKubeconfig(t, "one")
	fileConfig := writeKubeconfig(t, "two")

	tests := []struct {
		name    string
		options schema.OptionBlock
		host    string
		err     string
	}{
		{name: "no kubeconfig fails without opt-in", options: schema.OptionBlock{}, err: "no kubeconfig_file or kubeconfig_encoded provided"},
		{name: "empty kubeconfig_file fails without opt-in", options: schema.OptionBlock{"kubeconfig_file": ""}, err: "no kubeconfig_file or kubeconfig_encoded provided"},
		{name: "opt-in uses KUBECONFIG", options: schema.OptionBlock{"use_default_kubeconfig": "true"}, host: "one.example.test:6443"},
		{name: "opt-in honours context", options: schema.OptionBlock{"use_default_kubeconfig": "true", "context": "two"}, host: "two.example.test:6443"},
		{name: "explicit file ignores KUBECONFIG", options: schema.OptionBlock{"kubeconfig_file": fileConfig}, host: "two.example.test:6443"},
		{name: "explicit file honours context", options: schema.OptionBlock{"kubeconfig_file": fileConfig, "context": "one"}, host: "one.example.test:6443"},
		{name: "missing explicit file fails", options: schema.OptionBlock{"kubeconfig_file": filepath.Join(t.TempDir(), "missing")}, err: "could not read kubeconfig file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KUBERNETES_SERVICE_HOST", "")
			t.Setenv("KUBECONFIG", envConfig)

			p, err := New(tt.options)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.host, host(t, p))
		})
	}
}

func TestNewDefaultKubeconfigNotFound(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))

	_, err := New(schema.OptionBlock{"use_default_kubeconfig": "true"})
	require.ErrorContains(t, err, "no kubeconfig found")
}
