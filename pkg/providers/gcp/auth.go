package gcp

import (
	"context"
	"net/http"

	errorutil "github.com/projectdiscovery/utils/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"k8s.io/client-go/rest"
)

const (
	googleAuthPlugin = "gcp"
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

func register(ctx context.Context, serviceAccountKey []byte) (option.ClientOption, error) {
	var creds *google.Credentials

	if serviceAccountKey == nil {
		// If the service account key is not provided, use default credentials (https://cloud.google.com/docs/authentication/provide-credentials-adc)
		defaultCreds, err := google.FindDefaultCredentials(ctx, scope)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("failed to find default google credentials")
		}
		creds = defaultCreds
	} else {
		// If the service account key is not provided, use the service account key provided by the config
		saKeyCreds, err := google.CredentialsFromJSON(ctx, serviceAccountKey, scope)
		if err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("failed to parse specified GCP service account key")
		}
		creds = saKeyCreds
	}

	// Setup GKE auth using the token source from the credentials
	tokenSource := creds.TokenSource
	_ = rest.RegisterAuthProviderPlugin(googleAuthPlugin,
		func(clusterAddress string, config map[string]string, persister rest.AuthProviderConfigPersister) (rest.AuthProvider, error) {
			return &googleAuthProvider{tokenSource: tokenSource}, nil
		})

	return option.WithCredentials(creds), nil
}
