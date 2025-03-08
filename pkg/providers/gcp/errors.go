package gcp

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// googleRpcDetail is a minimal struct to map the objects inside the "Details" array.
// We only parse the fields we specifically need, such as "@type" and "reason."
type googleRpcDetail struct {
	Type   string `json:"@type"`
	Reason string `json:"reason"`
}

// ExtractGoogleErrorReason attempts to yield a concise error message
// by parsing Google API errors. We look for "Error 403" plus a "Details:"
// JSON array. We have also extended it to handle other HTTP codes or
// text-based checks such as "access denied."
func ExtractGoogleErrorReason(err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// Added: if it explicitly says "Access Denied" or "access denied" but no "Error 403":
	if strings.Contains(strings.ToLower(errMsg), "access denied") {
		return "Access denied (general). Possibly missing permissions or API not enabled."
	}

	// Added: parse other codes, e.g. Error 401 or Error 404
	// You could add more branches for other codes.
	if strings.Contains(errMsg, "Error 401") {
		return "Unauthorised request (401). Check your credentials."
	}
	if strings.Contains(errMsg, "Error 404") {
		return "Resource not found (404)."
	}

	// Original logic for 403 + googleapi
	if strings.Contains(errMsg, "Error 403") && strings.Contains(errMsg, "googleapi") {
		// Extract the main portion of the message (e.g. "Permission denied on resource project rosterfy-sbx.")
		mainMsg := extractMain403Message(errMsg)

		// Look for the JSON array after "Details:"
		jsonArray := extractDetailsArray(errMsg)
		if jsonArray == "" {
			// If we cannot find the details array, just return the main message
			return fmt.Sprintf("Access denied: %s", mainMsg)
		}

		// Attempt to parse the JSON array to find any "reason" fields
		reason := parseErrorReason(jsonArray)
		if reason != "" {
			// If we found a reason, add it
			return fmt.Sprintf("Access denied: %s (Reason: %s)", mainMsg, reason)
		}

		// If no reason found, return the main error text
		return fmt.Sprintf("Access denied: %s", mainMsg)
	}

	// If none of the above matches, do a fallback
	return fallbackErrorMessage(errMsg)
}

// FormatGoogleError now wraps the error using ExtractGoogleErrorReason
// so that any reason or code info is incorporated in the returned error string.
// We also log the final message to a file for review.
func FormatGoogleError(err error) error {
	if err == nil {
		return nil
	}
	finalMsg := ExtractGoogleErrorReason(err)

	// Log the final string to a file before returning.
	logErrorToFile(finalMsg)

	return fmt.Errorf("%s", finalMsg)
}

// fallbackErrorMessage returns a shortened version if no known parsing rules match.
// We can increase the limit from 150 to, say, 300 for a bit more detail.
func fallbackErrorMessage(errMsg string) string {
	if len(errMsg) > 300 {
		return errMsg[:297] + "..."
	}
	return errMsg
}

// extractMain403Message pulls out the text following "Error 403: ", up to a comma or newline.
func extractMain403Message(errMsg string) string {
	// Example:
	//   "googleapi: Error 403: Permission denied on resource project rosterfy-sbx., forbidden"
	r := regexp.MustCompile(`(?s)Error 403:\s+([^,\n]+)`)
	matches := r.FindStringSubmatch(errMsg)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return "Permission denied"
}

// extractDetailsArray finds the JSON segment immediately following "Details:", up to the next ']'.
func extractDetailsArray(errMsg string) string {
	// Expecting something like:
	//   Details:
	//   [
	//     { "@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "CONSUMER_INVALID" }
	//   ],
	r := regexp.MustCompile(`(?s)Details:\s*\[(.*?)\]`)
	matches := r.FindStringSubmatch(errMsg)
	if len(matches) > 1 {
		return fmt.Sprintf("[%s]", matches[1])
	}
	return ""
}

// parseErrorReason unpacks our mini struct and grabs the first non-empty "reason" it finds.
func parseErrorReason(jsonArray string) string {
	var detailItems []googleRpcDetail
	if err := json.Unmarshal([]byte(jsonArray), &detailItems); err != nil {
		return ""
	}
	for _, item := range detailItems {
		if item.Reason != "" {
			return item.Reason
		}
	}
	return ""
}

// logErrorToFile opens a file (creating it if needed) and appends the provided message.
// This is a simple demonstration; in a production setting, you may want to:
//   - keep this file open instead of re-opening for each log
//   - implement a concurrency-safe approach (like a sync.Mutex or a logging library)
func logErrorToFile(msg string) {
	f, err := os.OpenFile("gcp_error_logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		// If we cannot open the log file, just print to console as fallback
		log.Printf("Could not open error log file: %v", err)
		return
	}
	defer f.Close()

	logger := log.New(f, "", log.LstdFlags)
	logger.Println(msg)
}
