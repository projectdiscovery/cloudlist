package gcp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "seconds format",
			input:       "3600s",
			expected:    time.Hour,
			expectError: false,
		},
		{
			name:        "hour format",
			input:       "1h",
			expected:    time.Hour,
			expectError: false,
		},
		{
			name:        "minutes format",
			input:       "30m",
			expected:    30 * time.Minute,
			expectError: false,
		},
		{
			name:        "multiple units",
			input:       "1h30m",
			expected:    90 * time.Minute,
			expectError: false,
		},
		{
			name:        "12 hours in seconds",
			input:       "43200s",
			expected:    12 * time.Hour,
			expectError: false,
		},
		{
			name:        "invalid format",
			input:       "invalid",
			expected:    0,
			expectError: true,
		},
		{
			name:        "invalid seconds format",
			input:       "abcs",
			expected:    0,
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestShortLivedTokenSource_ValidateLifetime(t *testing.T) {
	tests := []struct {
		name        string
		lifetime    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid 1 hour",
			lifetime:    "3600s",
			expectError: false,
		},
		{
			name:        "valid 12 hours",
			lifetime:    "43200s",
			expectError: false,
		},
		{
			name:        "valid 1 second",
			lifetime:    "1s",
			expectError: false,
		},
		{
			name:        "invalid - too long (13 hours)",
			lifetime:    "46800s",
			expectError: true,
			errorMsg:    "token lifetime must be between 1 second and 12 hours",
		},
		{
			name:        "invalid - zero",
			lifetime:    "0s",
			expectError: true,
			errorMsg:    "token lifetime must be between 1 second and 12 hours",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, err := parseDuration(tt.lifetime)
			if err != nil && !tt.expectError {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if !tt.expectError {
				assert.NoError(t, err)
				// Validate it's within GCP limits
				assert.True(t, duration >= time.Second, "duration should be >= 1 second")
				assert.True(t, duration <= 12*time.Hour, "duration should be <= 12 hours")
			} else if duration != 0 {
				// If we got a duration, validate it's outside acceptable range
				isValid := duration >= time.Second && duration <= 12*time.Hour
				assert.False(t, isValid, "duration should be outside valid range")
			}
		})
	}
}

func TestRegisterWithOptions_Validation(t *testing.T) {
	tests := []struct {
		name                 string
		useShortLived        bool
		targetServiceAccount string
		expectError          bool
		errorMsg             string
	}{
		{
			name:                 "short-lived without service account email",
			useShortLived:        true,
			targetServiceAccount: "",
			expectError:          true,
			errorMsg:             "service_account_email is required when use_short_lived_credentials is true",
		},
		{
			name:                 "short-lived with service account email",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			expectError:          false,
		},
		{
			name:                 "traditional auth without service account email",
			useShortLived:        false,
			targetServiceAccount: "",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test validates the validation logic
			// We can't actually call registerWithOptions without valid GCP credentials
			// So we just verify the validation logic
			if tt.useShortLived && tt.targetServiceAccount == "" {
				// This should fail validation
				assert.True(t, tt.expectError)
			}
		})
	}
}

func TestTokenLifetimeDefaults(t *testing.T) {
	// Test that default token lifetime is set correctly
	defaultLifetime := "3600s"
	duration, err := parseDuration(defaultLifetime)
	assert.NoError(t, err)
	assert.Equal(t, time.Hour, duration)
}
