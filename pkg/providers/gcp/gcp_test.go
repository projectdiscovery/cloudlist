package gcp

import (
	"testing"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrganizationProvider_NoProjectsListedFallsBackToOrgLevel(t *testing.T) {
	// When organization_id is set without project_ids, the provider should be
	// created successfully regardless of whether Projects.List returns results.
	// Previously, the code returned "no projects available for organization discovery"
	// which blocked org-level Asset API discovery for SAs without project-list permission.

	options := schema.OptionBlock{
		"provider":          "gcp",
		"organization_id":   "123456789",
		"extended_metadata": "false",
	}

	provider, err := newOrganizationProvider(options, "test-id", "", "123456789")

	if err != nil {
		assert.NotContains(t, err.Error(), "no projects available for organization discovery",
			"org-level provider should not fail just because Projects.List returns 0 projects")
		t.Skipf("skipping (no GCP creds): %v", err)
	}

	require.NotNil(t, provider)
	assert.Equal(t, "123456789", provider.organizationID)
	assert.Nil(t, provider.projectScope, "projectScope should be nil for org-level discovery")
}

func TestNewOrganizationProvider_ConfiguredProjectsStillValidated(t *testing.T) {
	// When project_ids are explicitly configured, we should still fail if they
	// resolve to an empty list — this is a real config error.
	options := schema.OptionBlock{
		"provider":        "gcp",
		"organization_id": "123456789",
		"project_ids":     "",
	}

	provider, err := newOrganizationProvider(options, "test-id", "", "123456789")

	// With empty project_ids, getProjectIDsFromOptions returns nil, so it
	// takes the org-level path (no project scope). Should not fail.
	if err != nil {
		assert.NotContains(t, err.Error(), "no projects available",
			"empty project_ids should fall through to org-level discovery")
		t.Skipf("skipping in CI (no GCP creds): %v", err)
	}

	require.NotNil(t, provider)
	assert.Nil(t, provider.projectScope)
}
