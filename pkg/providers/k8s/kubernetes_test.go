package k8s

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/require"
)

const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://k8s.example.test:6443
users:
- name: test
  user:
    token: test
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
`

func isolateKubeconfig(t *testing.T, kubeconfig string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBECONFIG", kubeconfig)
}

func TestNewWithoutKubeconfigUsesEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(testKubeconfig), 0o600))
	isolateKubeconfig(t, path)

	p, err := New(schema.OptionBlock{})
	require.NoError(t, err)
	require.Equal(t, "k8s.example.test:6443", p.clientSet.CoreV1().RESTClient().Get().URL().Host)
}

func TestNewWithoutKubeconfigFailsWhenNoneFound(t *testing.T) {
	isolateKubeconfig(t, filepath.Join(t.TempDir(), "missing"))

	_, err := New(schema.OptionBlock{})
	require.Error(t, err)
}
