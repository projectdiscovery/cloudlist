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

func register(ctx context.Context, data []byte) (option.ClientOption, error) {
	// Register the auth plugin, it is requied for accessing GKE cluster using kubeconfig
	creds, err := google.CredentialsFromJSON(ctx, data, scope)
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("failed to create google credentials")
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("failed to create google token")
	}
	tokenSource := oauth2.StaticTokenSource(token)

	// Authenticate with the token
	// If it's nil use Google ADC
	err = rest.RegisterAuthProviderPlugin(googleAuthPlugin,
		func(clusterAddress string, config map[string]string, persister rest.AuthProviderConfigPersister) (rest.AuthProvider, error) {
			var err error
			if tokenSource == nil {
				tokenSource, err = google.DefaultTokenSource(ctx, scope)
				if err != nil {
					return nil, errorutil.NewWithErr(err).Msgf("failed to create google token source")
				}
			}
			return &googleAuthProvider{tokenSource: tokenSource}, nil
		})
	if err != nil {
		return nil, errorutil.NewWithErr(err).Msgf("failed to register %s auth plugin", googleAuthPlugin)
	}
	// return clioptions
	return option.WithCredentialsJSON(data), nil
}
