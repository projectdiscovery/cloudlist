package gcp

import (
	"testing"

	assetpb "cloud.google.com/go/asset/apiv1/assetpb"
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
