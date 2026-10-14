package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/projectdiscovery/gologger/writer"
	"github.com/stretchr/testify/require"
)

type logRecord struct {
	message string
	level   levels.Level
}

type captureWriter struct {
	records []logRecord
}

func (w *captureWriter) Write(data []byte, level levels.Level) {
	w.records = append(w.records, logRecord{message: string(data), level: level})
}

func captureDefaultLog(t *testing.T) *captureWriter {
	t.Helper()
	captured := &captureWriter{}
	gologger.DefaultLogger.SetMaxLevel(levels.LevelInfo)
	gologger.DefaultLogger.SetWriter(captured)
	t.Cleanup(func() {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelInfo)
		gologger.DefaultLogger.SetWriter(writer.NewCLI())
	})
	return captured
}

func skipMessage(records []logRecord) string {
	for _, rec := range records {
		if rec.level == levels.LevelInfo && strings.Contains(rec.message, "Skipping provider") {
			return rec.message
		}
	}
	return ""
}

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
	logs := captureDefaultLog(t)

	inv, err := r.newInventory(schema.Options{
		{"provider": "kubernetes", "kubeconfig_file": filepath.Join(t.TempDir(), "missing")},
		{"provider": "kubernetes", "kubeconfig_file": kubeconfig},
	})
	require.NoError(t, err)
	require.Len(t, inv.Providers, 1)
	skip := skipMessage(logs.records)
	require.Contains(t, skip, "could not create provider kubernetes")

	logs.records = nil
	_, err = r.newInventory(schema.Options{
		{"provider": "kubernetes", "kubeconfig_file": filepath.Join(t.TempDir(), "missing")},
	})
	require.ErrorContains(t, err, "no provider could be initialized")
	require.ErrorContains(t, err, "could not create provider kubernetes")
	require.Empty(t, skipMessage(logs.records))
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
