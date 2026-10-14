package gcp

import (
	"testing"

	assetpb "cloud.google.com/go/asset/apiv1/assetpb"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitAndCleanProjectList(t *testing.T) {
	input := "demo-project, prod-project, demo-project, ,\n qa-project"
	expected := []string{"demo-project", "prod-project", "qa-project"}

	result := splitAndCleanProjectList(input)

	require.Equal(t, expected, result)
}

func TestProjectScopeAllowsAssetByID(t *testing.T) {
	scope := newProjectScope([]string{"demo-project", "prod-project"})
	require.NotNil(t, scope)

	allowedAsset := &assetpb.Asset{
		Name: "//compute.googleapis.com/projects/demo-project/zones/us-central1-a/instances/test-instance",
	}

	blockedAsset := &assetpb.Asset{
		Name: "//compute.googleapis.com/projects/other-project/zones/us-central1-a/instances/test-instance",
	}

	assert.True(t, scope.allowsAsset(allowedAsset))
	assert.False(t, scope.allowsAsset(blockedAsset))
}

func TestProjectScopeAllowsAssetByNumber(t *testing.T) {
	scope := newProjectScope([]string{"123456789"})
	require.NotNil(t, scope)

	asset := &assetpb.Asset{
		Resource: &assetpb.Resource{
			Parent: "//cloudresourcemanager.googleapis.com/projects/123456789",
		},
	}

	assert.True(t, scope.allowsAsset(asset))
}

func TestGetExcludeProjectIDsFromOptions(t *testing.T) {
	t.Run("returns nil when not set", func(t *testing.T) {
		options := schema.OptionBlock{}
		result := getExcludeProjectIDsFromOptions(options)
		assert.Nil(t, result)
	})

	t.Run("parses comma-separated list", func(t *testing.T) {
		options := schema.OptionBlock{"exclude_project_ids": "project-a,project-b,project-c"}
		result := getExcludeProjectIDsFromOptions(options)
		assert.Equal(t, []string{"project-a", "project-b", "project-c"}, result)
	})

	t.Run("deduplicates entries", func(t *testing.T) {
		options := schema.OptionBlock{"exclude_project_ids": "project-a,project-a,project-b"}
		result := getExcludeProjectIDsFromOptions(options)
		assert.Equal(t, []string{"project-a", "project-b"}, result)
	})
}

func TestExcludeProjectValidation(t *testing.T) {
	t.Run("errors when both include and exclude set", func(t *testing.T) {
		options := schema.OptionBlock{
			"provider":            "gcp",
			"organization_id":     "123456",
			"project_ids":         "project-a",
			"exclude_project_ids": "project-b",
		}
		_, err := New(options)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("errors when exclude set without organization_id", func(t *testing.T) {
		options := schema.OptionBlock{
			"provider":            "gcp",
			"exclude_project_ids": "project-a",
		}
		_, err := New(options)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires organization_id")
	})
}

func TestExcludeProjectFiltering(t *testing.T) {
	allProjects := []string{"project-a", "project-b", "project-c", "project-d"}
	excludeScope := newProjectScope([]string{"project-b", "project-d"})
	require.NotNil(t, excludeScope)

	filtered := make([]string, 0, len(allProjects))
	for _, p := range allProjects {
		if !excludeScope.containsID(p) {
			filtered = append(filtered, p)
		}
	}

	assert.Equal(t, []string{"project-a", "project-c"}, filtered)
}
