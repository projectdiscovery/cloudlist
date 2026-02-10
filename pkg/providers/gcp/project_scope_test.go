package gcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitAndCleanProjectList(t *testing.T) {
	// YAML parsing converts list to comma-separated string
	input := "demo-project, prod-project, demo-project, , qa-project"
	expected := []string{"demo-project", "prod-project", "qa-project"}

	result := splitAndCleanProjectList(input)

	require.Equal(t, expected, result)
}

func TestNewProjectScope(t *testing.T) {
	scope := newProjectScope([]string{"demo-project", "prod-project", "demo-project"})
	require.NotNil(t, scope)
	require.Equal(t, []string{"demo-project", "prod-project"}, scope.listIDs())
}

func TestNewProjectScopeEmpty(t *testing.T) {
	scope := newProjectScope([]string{"", "  ", ""})
	require.Nil(t, scope)
}
