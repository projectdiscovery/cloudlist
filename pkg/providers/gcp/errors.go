package gcp

import (
	"fmt"
	"regexp"
	"strings"
)

// ExtractGoogleErrorReason extracts a concise reason from Google API errors
func ExtractGoogleErrorReason(err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Check for the standard Google API error pattern
	if strings.Contains(errMsg, "Error ") && strings.Contains(errMsg, "Details:") {
		// Extract the status message
		statusMatch := regexp.MustCompile(`Error \d+: (.+?)(\.?\s+Details:)`).FindStringSubmatch(errMsg)
		if len(statusMatch) > 1 {
			status := statusMatch[1]

			// Look for the "reason" field in the Details JSON
			reasonMatch := regexp.MustCompile(`"reason"\s*:\s*"([^"]+)"`).FindStringSubmatch(errMsg)
			if len(reasonMatch) > 1 {
				reason := reasonMatch[1]
				return fmt.Sprintf("%s (%s)", status, reason)
			}
			return status
		}
	}

	// If we couldn't parse it, return the original error message
	return errMsg
}

// FormatGoogleError is a simple pass-through function to keep compatibility
// with existing code using it
func FormatGoogleError(err error) error {
	// Simply return the error as-is
	return err
}
