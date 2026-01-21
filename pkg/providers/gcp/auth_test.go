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
			name:        "1 hour in seconds (max)",
			input:       "3600s",
			expected:    time.Hour,
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
			name:        "valid 1 hour (max)",
			lifetime:    "3600s",
			expectError: false,
		},
		{
			name:        "valid 1 second",
			lifetime:    "1s",
			expectError: false,
		},
		{
			name:        "invalid - too long (2 hours)",
			lifetime:    "7200s",
			expectError: true,
			errorMsg:    "token lifetime must be between 1 second and 1 hour (3600s)",
		},
		{
			name:        "invalid - zero",
			lifetime:    "0s",
			expectError: true,
			errorMsg:    "token lifetime must be between 1 second and 1 hour (3600s)",
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
				assert.True(t, duration <= time.Hour, "duration should be <= 1 hour")
			} else if duration != 0 {
				// If we got a duration, validate it's outside acceptable range
				isValid := duration >= time.Second && duration <= time.Hour
				assert.False(t, isValid, "duration should be outside valid range")
			}
		})
	}
}

func TestValidateShortLivedConfig(t *testing.T) {
	tests := []struct {
		name                 string
		useShortLived        bool
		targetServiceAccount string
		tokenLifetime        string
		expectError          bool
		errorContains        string
	}{
		{
			name:                 "traditional auth - no validation needed",
			useShortLived:        false,
			targetServiceAccount: "",
			tokenLifetime:        "",
			expectError:          false,
		},
		{
			name:                 "short-lived without service account email",
			useShortLived:        true,
			targetServiceAccount: "",
			tokenLifetime:        "3600s",
			expectError:          true,
			errorContains:        "service_account_email is required",
		},
		{
			name:                 "short-lived with valid config",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			tokenLifetime:        "3600s",
			expectError:          false,
		},
		{
			name:                 "short-lived with valid token lifetime (1h)",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			tokenLifetime:        "1h",
			expectError:          false,
		},
		{
			name:                 "short-lived with invalid token lifetime (too long)",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			tokenLifetime:        "7200s",
			expectError:          true,
			errorContains:        "token lifetime must be between 1 second and 1 hour",
		},
		{
			name:                 "short-lived with invalid token lifetime (zero)",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			tokenLifetime:        "0s",
			expectError:          true,
			errorContains:        "token lifetime must be between 1 second and 1 hour",
		},
		{
			name:                 "short-lived with invalid token lifetime format",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			tokenLifetime:        "invalid",
			expectError:          true,
			errorContains:        "invalid token lifetime",
		},
		{
			name:                 "short-lived with minimum valid lifetime (1s)",
			useShortLived:        true,
			targetServiceAccount: "test@project.iam.gserviceaccount.com",
			tokenLifetime:        "1s",
			expectError:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShortLivedConfig(tt.useShortLived, tt.targetServiceAccount, tt.tokenLifetime)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
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
