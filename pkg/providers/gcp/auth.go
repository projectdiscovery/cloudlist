package gcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/utils/errkit"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"
	"k8s.io/client-go/rest"
)

const (
	googleAuthPlugin = "gcp-cloudlist"
	scope            = "https://www.googleapis.com/auth/cloud-platform"
)

type googleAuthProvider struct {
	tokenSource oauth2.TokenSource
}

// These funcitons are needed even if we don't utilize them
// So that googleAuthProvider is an rest.AuthProvider interface
func (g *googleAuthProvider) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return &oauth2.Transport{
		Base:   rt,
		Source: g.tokenSource,
	}
}

func (g *googleAuthProvider) Login() error { return nil }

func (g *googleAuthProvider) Name() string { return googleAuthPlugin }

// shortLivedTokenSource implements oauth2.TokenSource for generating short-lived tokens
type shortLivedTokenSource struct {
	ctx                  context.Context
	sourceCredentials    *google.Credentials
	targetServiceAccount string
	lifetime             string
}

// Token generates a new short-lived access token using the IAM Service Account Credentials API
func (s *shortLivedTokenSource) Token() (*oauth2.Token, error) {
	// Create IAM Credentials API client
	iamService, err := iamcredentials.NewService(s.ctx, option.WithCredentials(s.sourceCredentials))
	if err != nil {
		return nil, errkit.Wrap(err, "failed to create IAM credentials service")
	}

	// Parse lifetime duration
	lifetimeDuration, err := parseDuration(s.lifetime)
	if err != nil {
		return nil, errkit.Wrap(err, "invalid token lifetime")
	}

	// Validate lifetime is within GCP limits (1 second to 1 hour)
	if lifetimeDuration < time.Second || lifetimeDuration > time.Hour {
		return nil, errkit.New("token lifetime must be between 1 second and 1 hour (3600s)")
	}

	// Prepare the request
	name := fmt.Sprintf("projects/-/serviceAccounts/%s", s.targetServiceAccount)
	req := &iamcredentials.GenerateAccessTokenRequest{
		Scope:    []string{scope},
		Lifetime: fmt.Sprintf("%ds", int(lifetimeDuration.Seconds())),
	}

	gologger.Debug().Msgf("Generating short-lived token for service account: %s (lifetime: %s)", s.targetServiceAccount, s.lifetime)

	// Generate the access token
	resp, err := iamService.Projects.ServiceAccounts.GenerateAccessToken(name, req).Context(s.ctx).Do()
	if err != nil {
		// Provide helpful error messages for common issues
		errMsg := err.Error()
		if strings.Contains(errMsg, "403") {
			return nil, errkit.Wrap(err, "permission denied: ensure source credentials have 'roles/iam.serviceAccountTokenCreator' or 'iam.serviceAccounts.generateAccessToken' permission on target service account")
		}
		if strings.Contains(errMsg, "404") {
			return nil, errkit.Wrap(err, "service account not found: verify the service_account_email is correct")
		}
		if strings.Contains(errMsg, "PERMISSION_DENIED") || strings.Contains(errMsg, "IAM_PERMISSION_DENIED") {
			return nil, errkit.Wrap(err, "IAM Service Account Credentials API may not be enabled or accessible")
		}
		return nil, errkit.Wrap(err, "failed to generate access token")
	}

	// Parse expiry time
	expiry, err := time.Parse(time.RFC3339, resp.ExpireTime)
	if err != nil {
		// If parsing fails, calculate expiry from now + lifetime
		expiry = time.Now().Add(lifetimeDuration)
	}

	gologger.Debug().Msgf("Successfully generated short-lived token, expires at: %s", expiry.Format(time.RFC3339))

	return &oauth2.Token{
		AccessToken: resp.AccessToken,
		TokenType:   "Bearer",
		Expiry:      expiry,
	}, nil
}

// parseDuration parses a duration string (e.g., "3600s", "1h") into time.Duration
func parseDuration(lifetime string) (time.Duration, error) {
	// Handle seconds format (e.g., "3600s")
	if strings.HasSuffix(lifetime, "s") {
		seconds, err := strconv.Atoi(strings.TrimSuffix(lifetime, "s"))
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", lifetime)
		}
		return time.Duration(seconds) * time.Second, nil
	}

	// Handle standard Go duration format (e.g., "1h", "30m")
	duration, err := time.ParseDuration(lifetime)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %s (use formats like '3600s' or '1h')", lifetime)
	}
	return duration, nil
}

func register(ctx context.Context, serviceAccountKey []byte) (option.ClientOption, error) {
	return registerWithOptions(ctx, serviceAccountKey, false, "", "", "3600s")
}

// registerWithOptions provides full control over authentication, including short-lived credentials
func registerWithOptions(
	ctx context.Context,
	serviceAccountKey []byte,
	useShortLived bool,
	targetServiceAccount string,
	sourceCredentialsPath string,
	tokenLifetime string,
) (option.ClientOption, error) {
	// Step 1: Obtain source credentials
	var sourceCreds *google.Credentials
	var err error

	if sourceCredentialsPath != "" {
		// Use explicitly provided source credentials file
		gologger.Debug().Msgf("Loading source credentials from: %s", sourceCredentialsPath)
		sourceKeyData, err := os.ReadFile(sourceCredentialsPath)
		if err != nil {
			return nil, errkit.Wrap(err, "failed to read source credentials file")
		}
		sourceCreds, err = google.CredentialsFromJSON(ctx, sourceKeyData, scope)
		if err != nil {
			return nil, errkit.Wrap(err, "failed to parse source credentials")
		}
	} else if len(serviceAccountKey) > 0 {
		// Use service account key from config
		gologger.Debug().Msgf("Using service account key from configuration")
		sourceCreds, err = google.CredentialsFromJSON(ctx, serviceAccountKey, scope)
		if err != nil {
			return nil, errkit.Wrap(err, "failed to parse specified GCP service account key")
		}
	} else {
		// Fall back to Application Default Credentials (ADC)
		gologger.Debug().Msgf("Using Application Default Credentials (ADC)")
		sourceCreds, err = google.FindDefaultCredentials(ctx, scope)
		if err != nil {
			return nil, errkit.Wrap(err, "failed to find default google credentials")
		}
	}

	// Step 2: If short-lived credentials are requested, generate them
	var finalTokenSource oauth2.TokenSource
	if useShortLived {
		if targetServiceAccount == "" {
			return nil, errkit.New("service_account_email is required when use_short_lived_credentials is true")
		}

		gologger.Info().Msgf("Configuring short-lived credentials for service account: %s", targetServiceAccount)

		// Create short-lived token source
		shortLivedSource := &shortLivedTokenSource{
			ctx:                  ctx,
			sourceCredentials:    sourceCreds,
			targetServiceAccount: targetServiceAccount,
			lifetime:             tokenLifetime,
		}

		// Wrap with ReuseTokenSource for automatic caching and refresh
		finalTokenSource = oauth2.ReuseTokenSource(nil, shortLivedSource)

		// Test token generation immediately to catch configuration errors early
		_, err := finalTokenSource.Token()
		if err != nil {
			return nil, errkit.Wrap(err, "failed to generate initial short-lived token")
		}

		gologger.Info().Msgf("Successfully configured short-lived credentials (lifetime: %s)", tokenLifetime)
	} else {
		// Use source credentials directly (traditional approach)
		finalTokenSource = sourceCreds.TokenSource
	}

	// Step 3: Setup GKE auth using the final token source
	err = rest.RegisterAuthProviderPlugin(googleAuthPlugin,
		func(clusterAddress string, config map[string]string, persister rest.AuthProviderConfigPersister) (rest.AuthProvider, error) {
			return &googleAuthProvider{tokenSource: finalTokenSource}, nil
		})
	if err != nil {
		return nil, errkit.Wrap(err, "failed to register GKE auth provider plugin")
	}

	// Step 4: Return appropriate client option
	if useShortLived {
		// For short-lived tokens, return token source directly
		return option.WithTokenSource(finalTokenSource), nil
	} else {
		// For traditional auth, return credentials object
		return option.WithCredentials(sourceCreds), nil
	}
}
