package gcp

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractGoogleErrorReason(t *testing.T) {
	// This string simulates a GCP 403 error that includes an embedded JSON array after "Details:"
	mockErrString := `googleapi: Error 403: Permission denied on resource project test-project, forbidden
Details:
[
  {
    "@type": "type.googleapis.com/google.rpc.ErrorInfo",
    "reason": "CONSUMER_INVALID"
  }
]`

	result := ExtractGoogleErrorReason(errors.New(mockErrString))
	expected := "Access denied: Permission denied on resource project test-project (Reason: CONSUMER_INVALID)"

	if result != expected {
		t.Errorf("expected:\n  %q\ngot:\n  %q", expected, result)
	}
}

func TestFormatGoogleError(t *testing.T) {
	// Same mock error as above. Now check FormatGoogleError directly.
	mockErrString := `googleapi: Error 403: Permission denied on resource project test-project, forbidden
Details:
[
  {
    "@type": "type.googleapis.com/google.rpc.ErrorInfo",
    "reason": "CONSUMER_INVALID"
  }
]`

	err := FormatGoogleError(errors.New(mockErrString))
	result := err.Error()
	expected := "Access denied: Permission denied on resource project test-project (Reason: CONSUMER_INVALID)"

	if result != expected {
		t.Errorf("expected:\n  %q\ngot:\n  %q", expected, result)
	}
}

func TestExtractGoogleErrorReasonWithFallback(t *testing.T) {
	// This string simulates a GCP 403 error that includes an embedded JSON array after "Details:"
	mockErrString := `googleapi: Error 403: Permission denied on resource project test-project, forbidden
Details:
[
  {
    "@type": "type.googleapis.com/google.rpc.ErrorInfo",
    "reason": "CONSUMER_INVALID"
  }
]`

	result := ExtractGoogleErrorReason(errors.New(mockErrString))
	expected := "Access denied: Permission denied on resource project test-project (Reason: CONSUMER_INVALID)"

	if result != expected {
		t.Errorf("expected:\n  %q\ngot:\n  %q", expected, result)
	}

	// If an error line says "Error 401" or "Error 404" etc.:
	if strings.Contains(mockErrString, "Error 401") {
		// Return something like "Access unauthorised…"
		t.Errorf("Access unauthorised…")
	} else if strings.Contains(mockErrString, "Error 404") {
		// Return "Resource not found…"
		t.Errorf("Resource not found…")
	} else if strings.Contains(mockErrString, "Error 405") {
		// Return "Method not allowed…"
		t.Errorf("Method not allowed…")
	} else if strings.Contains(mockErrString, "Error 406") {
		// Return "Not acceptable…"
		t.Errorf("Not acceptable…")
	} else if strings.Contains(mockErrString, "Error 407") {
		// Return "Proxy authentication required…"
		t.Errorf("Proxy authentication required…")
	} else if strings.Contains(mockErrString, "Error 408") {
		// Return "Request timeout…"
		t.Errorf("Request timeout…")
	} else if strings.Contains(mockErrString, "Error 409") {
		// Return "Conflict…"
		t.Errorf("Conflict…")
	} else if strings.Contains(mockErrString, "Error 410") {
		// Return "Gone…"
		t.Errorf("Gone…")
	} else if strings.Contains(mockErrString, "Error 411") {
		// Return "Length required…"
		t.Errorf("Length required…")
	} else if strings.Contains(mockErrString, "Error 412") {
		// Return "Precondition failed…"
		t.Errorf("Precondition failed…")
	} else if strings.Contains(mockErrString, "Error 413") {
		// Return "Request entity too large…"
		t.Errorf("Request entity too large…")
	} else if strings.Contains(mockErrString, "Error 414") {
		// Return "Request-URI too long…"
		t.Errorf("Request-URI too long…")
	} else if strings.Contains(mockErrString, "Error 415") {
		// Return "Unsupported media type…"
		t.Errorf("Unsupported media type…")
	} else if strings.Contains(mockErrString, "Error 416") {
		// Return "Requested range not satisfiable…"
		t.Errorf("Requested range not satisfiable…")
	} else if strings.Contains(mockErrString, "Error 417") {
		// Return "Expectation failed…"
		t.Errorf("Expectation failed…")
	} else if strings.Contains(mockErrString, "Error 418") {
		// Return "I'm a teapot…"
		t.Errorf("I'm a teapot…")
	} else if strings.Contains(mockErrString, "Error 421") {
		// Return "Misdirected request…"
		t.Errorf("Misdirected request…")
	} else if strings.Contains(mockErrString, "Error 422") {
		// Return "Unprocessable entity…"
		t.Errorf("Unprocessable entity…")
	} else if strings.Contains(mockErrString, "Error 423") {
		// Return "Locked…"
		t.Errorf("Locked…")
	} else if strings.Contains(mockErrString, "Error 424") {
		// Return "Failed dependency…"
		t.Errorf("Failed dependency…")
	} else if strings.Contains(mockErrString, "Error 425") {
		// Return "Too early…"
		t.Errorf("Too early…")
	} else if strings.Contains(mockErrString, "Error 426") {
		// Return "Upgrade required…"
		t.Errorf("Upgrade required…")
	} else if strings.Contains(mockErrString, "Error 428") {
		// Return "Precondition required…"
		t.Errorf("Precondition required…")
	} else if strings.Contains(mockErrString, "Error 429") {
		// Return "Too many requests…"
		t.Errorf("Too many requests…")
	} else if strings.Contains(mockErrString, "Error 431") {
		// Return "Request header fields too large…"
		t.Errorf("Request header fields too large…")
	} else if strings.Contains(mockErrString, "Error 451") {
		// Return "Unavailable for legal reasons…"
		t.Errorf("Unavailable for legal reasons…")
	} else if strings.Contains(mockErrString, "Error 500") {
		// Return "Internal server error…"
		t.Errorf("Internal server error…")
	} else if strings.Contains(mockErrString, "Error 501") {
		// Return "Not implemented…"
		t.Errorf("Not implemented…")
	} else if strings.Contains(mockErrString, "Error 502") {
		// Return "Bad gateway…"
		t.Errorf("Bad gateway…")
	} else if strings.Contains(mockErrString, "Error 503") {
		// Return "Service unavailable…"
		t.Errorf("Service unavailable…")
	} else if strings.Contains(mockErrString, "Error 504") {
		// Return "Gateway timeout…"
		t.Errorf("Gateway timeout…")
	} else if strings.Contains(mockErrString, "Error 505") {
		// Return "HTTP version not supported…"
		t.Errorf("HTTP version not supported…")
	} else if strings.Contains(mockErrString, "Error 506") {
		// Return "Variant also negotiates…"
		t.Errorf("Variant also negotiates…")
	} else if strings.Contains(mockErrString, "Error 507") {
		// Return "Insufficient storage…"
		t.Errorf("Insufficient storage…")
	} else if strings.Contains(mockErrString, "Error 508") {
		// Return "Loop detected…"
		t.Errorf("Loop detected…")
	} else if strings.Contains(mockErrString, "Error 510") {
		// Return "Not extended…"
		t.Errorf("Not extended…")
	} else if strings.Contains(mockErrString, "Error 511") {
		// Return "Network authentication required…"
		t.Errorf("Network authentication required…")
	}
}
