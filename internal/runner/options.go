package runner

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/goflags"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/levels"
	fileutil "github.com/projectdiscovery/utils/file"
	updateutils "github.com/projectdiscovery/utils/update"
	"gopkg.in/yaml.v2"
)

// Options contains the configuration options for cloudlist.
type Options struct {
	JSON               bool                // JSON returns JSON output
	Silent             bool                // Silent Display results only
	Version            bool                // Version returns the version of the tool.
	Verbose            bool                // Verbose prints verbose output.
	Hosts              bool                // Hosts specifies to fetch only DNS Names
	IPAddress          bool                // IPAddress specifes to fetch only IP Addresses
	Config             string              // Config is the location of the config file.
	Output             string              // Output is the file to write found results too.
	ExcludePrivate     bool                // ExcludePrivate excludes private IPs from results
	Provider           goflags.StringSlice // Provider specifies what providers to fetch assets for.
	Id                 goflags.StringSlice // Id specifies what id's to fetch assets for.
	ProviderConfig     string              // ProviderConfig is the location of the provider config file.
	DisableUpdateCheck bool                // DisableUpdateCheck disable automatic update check
}

var (
	defaultConfigLocation         = filepath.Join(userHomeDir(), ".config/cloudlist/config.yaml")
	defaultProviderConfigLocation = filepath.Join(userHomeDir(), ".config/cloudlist/provider-config.yaml")
)

// ParseOptions parses the command line flags provided by a user
func ParseOptions() *Options {
	// Migrate config to provider config
	if fileutil.FileExists(defaultConfigLocation) && !fileutil.FileExists(defaultProviderConfigLocation) {
		if _, err := readProviderConfig(defaultConfigLocation); err == nil {
			gologger.Info().Msg("Detected old config.yaml file, trying to rename it to provider-config.yaml\n")
			if err := os.Rename(defaultConfigLocation, defaultProviderConfigLocation); err != nil {
				gologger.Fatal().Msgf("Could not rename existing config (config.yaml) to provider config (provider-config.yaml): %s\n", err)
			} else {
				gologger.Info().Msg("Renamed config.yaml to provider-config.yaml successfully\n")
			}
		}
	}

	options := &Options{}
	flagSet := goflags.NewFlagSet()
	flagSet.SetDescription(`Cloudlist is a tool for listing Assets from multiple cloud providers.`)

	flagSet.CreateGroup("config", "Configuration",
		flagSet.StringVar(&options.Config, "config", defaultConfigLocation, "cloudlist flag config file"),
		flagSet.StringVarP(&options.ProviderConfig, "provider-config", "pc", defaultProviderConfigLocation, "provider config file"),
	)
	flagSet.CreateGroup("filter", "Filters",
		flagSet.StringSliceVarP(&options.Provider, "provider", "p", nil, "display results for given providers (comma-separated)", goflags.NormalizedStringSliceOptions),
		flagSet.StringSliceVar(&options.Id, "id", nil, "display results for given ids (comma-separated)", goflags.NormalizedStringSliceOptions),
		flagSet.BoolVar(&options.Hosts, "host", false, "display only hostnames in results"),
		flagSet.BoolVar(&options.IPAddress, "ip", false, "display only ips in results"),
		flagSet.BoolVarP(&options.ExcludePrivate, "exclude-private", "ep", false, "exclude private ips in cli output"),
	)
	flagSet.CreateGroup("update", "Update",
		flagSet.CallbackVarP(GetUpdateCallback(), "update", "up", "update cloudlist to latest version"),
		flagSet.BoolVarP(&options.DisableUpdateCheck, "disable-update-check", "duc", false, "disable automatic cloudlist update check"),
	)
	flagSet.CreateGroup("output", "Output",
		flagSet.StringVarP(&options.Output, "output", "o", "", "output file to write results"),
		flagSet.BoolVar(&options.JSON, "json", false, "write output in json format"),
		flagSet.BoolVar(&options.Version, "version", false, "display version of cloudlist"),
		flagSet.BoolVar(&options.Verbose, "v", false, "display verbose output"),
		flagSet.BoolVar(&options.Silent, "silent", false, "display only results in output"),
	)

	_ = flagSet.Parse()

	options.configureOutput()
	showBanner()
	if options.Version {
		gologger.Info().Msgf("Current Version: %s\n", version)
		os.Exit(0)
	}

	if !options.DisableUpdateCheck {
		latestVersion, err := updateutils.GetVersionCheckCallback("cloudlist")()
		if err != nil {
			if options.Verbose {
				gologger.Error().Msgf("cloudlist version check failed: %v", err.Error())
			}
		} else {
			gologger.Info().Msgf("Current cloudlist version %v %v", version, updateutils.GetVersionDescription(version, latestVersion))
		}
	}

	checkAndCreateProviderConfigFile(options)
	return options
}

// configureOutput configures the output on the screen
func (options *Options) configureOutput() {
	// If the user desires verbose output, show verbose output
	if options.Verbose {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelVerbose)
	}
	if options.Silent {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelSilent)
	}
}

// readProviderConfig reads the provider config file from the options
func readProviderConfig(configFile string) (schema.Options, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := schema.Options{}
	if err := yaml.NewDecoder(file).Decode(&config); err != nil {
		if err == io.EOF {
			return nil, errors.New("invalid provider configuration file provided")
		}
		return nil, err
	}
	return config, nil
}

// checkAndCreateProviderConfigFile checks if a provider config file exists,
// if not creates a default.
func checkAndCreateProviderConfigFile(options *Options) {
	if options.ProviderConfig == "" || !fileutil.FileExists(defaultProviderConfigLocation) {
		err := os.MkdirAll(filepath.Dir(options.ProviderConfig), os.ModePerm)
		if err != nil {
			gologger.Warning().Msgf("Could not create default config file: %s\n", err)
		}
		if !fileutil.FileExists(defaultProviderConfigLocation) {
			if writeErr := ioutil.WriteFile(defaultProviderConfigLocation, []byte(defaultProviderConfigFile), os.ModePerm); writeErr != nil {
				gologger.Warning().Msgf("Could not write default output to %s: %s\n", defaultProviderConfigLocation, writeErr)
			}
		}
	}
}

func userHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		gologger.Fatal().Msgf("Could not get user home directory: %s\n", err)
	}
	return usr.HomeDir
}

const defaultProviderConfigFile = `#  #Provider configuration file for cloudlist enumeration agent

#- # provider is the name of the provider
#  provider: do
#  # id is the name of the provider id
#  id: xxxx
#  # digitalocean_token is the API key for digitalocean cloud platform
#  digitalocean_token: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
#
#- # provider is the name of the provider
#  provider: scw
#  # scaleway_access_key is the access key for scaleway API
#  scaleway_access_key: SCWXXXXXXXXXXXXXX
#  # scaleway_access_token is the access token for scaleway API
#  scaleway_access_token: xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxx
#
#- # provider is the name of the provider
#  provider: aws
#  # id is the name of the provider id
#  id: staging
#  # aws_access_key is the access key for AWS account
#  aws_access_key: AKIAXXXXXXXXXXXXXX
#  # aws_secret_key is the secret key for AWS account
#  aws_secret_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
#  # aws_session_token session token for temporary security credentials retrieved via STS (optional)
#  aws_session_token: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

#- # provider is the name of the provider
#  provider: azure
#  # id is the name of the provider id
#  id: staging
#  # client_id is the client ID of registered application of the azure account (not requuired if using cli auth)
#  client_id: xxxxxxxxxxxxxxxxxxxxxxxxx
#  # client_secret is the secret ID of registered application of the zure account (not requuired if using cli auth)
#  client_secret: xxxxxxxxxxxxxxxxxxxxx
#  # tenant_id is the tenant ID of registered application of the azure account (not requuired if using cli auth)
#  tenant_id: xxxxxxxxxxxxxxxxxxxxxxxxx
#  #subscription_id is the azure subscription id
#  subscription_id: xxxxxxxxxxxxxxxxxxx
#  #use_cli_auth if set to true cloudlist will use azure cli auth
#  use_cli_auth: true
 
#- # provider is the name of the provider
#  provider: cloudflare
#  # email is the email for cloudflare
#  email: user@domain.com
#  # api_key is the api_key for cloudflare
#  api_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
#  # api_token is the scoped_api_token for cloudflare (optional)
#  api_token: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

#- # provider is the name of the provider
#  provider: heroku
#  # id is the name of the provider id
#  id: staging
#  # heroku_api_token is the api key for Heroku account
#  heroku_api_token: xxxxxxxxxxxxxxxxxxxx

#- # provider is the name of the provider
#  provider: linode
#  # id is the name of the provider id
#  id: staging
#  # linode_personal_access_token is the personal access token for linode account
#  linode_personal_access_token: XXXXXXXXXXXXXXXXXXXXXXXX

#- # provider is the name of the provider
#  provider: fastly
#  # id is the name of the provider id
#  id: staging
#  # fastly_api_key is the personal API token for fastly account
#  fastly_api_key: XX-XXXXXXXXXXXXXXXXXXXXXX-

#- # provider is the name of the provider
#  provider: alibaba
#  # id is the name of the provider id
#  id: staging
#  # alibaba_region_id is the region id of the resources
#  alibaba_region_id: ap-XXXXXXX
#  # alibaba_access_key is the access key ID for alibaba cloud account
#  alibaba_access_key: XXXXXXXXXXXXXXXXXXXX
#  # alibaba_access_key_secret is the secret access key for alibaba cloud account
#  alibaba_access_key_secret: XXXXXXXXXXXXXXXX

# - # provider is the name of the provider
#  provider: namecheap
#  # id is the name of the provider id
#  id: staging
#  # namecheap_api_key is the api key for namecheap account
#  namecheap_api_key: XXXXXXXXXXXXXXXXXXXXXXX
#  # namecheap_user_name is the username of the namecheap account
#  namecheap_user_name: XXXXXXX

# - # provider is the name of the provider
#  provider: terraform
#  # id is the name of the provider id
#  id: staging
#  #tf_state_file is the location of terraform state file (terraform.tfsate) 
#  tf_state_file: path/to/terraform.tfstate

# - # provider is the name of the provider
# provider: nomad
#  # nomad_url is the url for nomad server
#  nomad_url: http:/127.0.0.1:4646/
#  # nomad_ca_file is the path to nomad CA file
#  # nomad_ca_file: <path-to-ca-file>.pem
#  # nomad_cert_file is the path to nomad Certificate file
#  # nomad_cert_file: <path-to-cert-file>.pem
#  # nomad_key_file is the path to nomad Certificate Key file
#  # nomad_key_file: <path-to-key-file>.pem
#  # nomad_token is the nomad authentication token
#  # nomad_token: <nomad-token>
#  # nomad_http_auth is the nomad http auth value
#  # nomad_http_auth: <nomad-http-auth-value>

#- # provider is the name of the provider
#  provider: consul
#  # consul_url is the url for consul server
#  consul_url: http://localhost:8500/
#  # consul_ca_file is the path to consul CA file
#  # consul_ca_file: <path-to-ca-file>.pem
#  # consul_cert_file is the path to consul Certificate file
#  # consul_cert_file: <path-to-cert-file>.pem
#  # consul_key_file is the path to consul Certificate Key file
#  # consul_key_file: <path-to-key-file>.pem
#  # consul_http_token is the consul authentication token
#  # consul_http_token: <consul-token>
#  # consul_http_auth is the consul http auth value
#  # consul_http_auth: <consul-http-auth-value>

#- # provider is the name of the provider
#  provider: hetzner
#  # id is the name of the provider id
#  id: staging
#  # auth_token is the is the hetzner authentication token
#  auth_token: <hetzner-token>

#- # provider is the name of the provider
#  provider: openstack
#  # id is the name of the provider id
#  id: staging
#  # identity_endpoint is Openstack identity endpoint used to authenticate
#  identity_endpoint: <openstack-identity-endpoint>
#  # domain_name is Openstack domain name used to authenticate
#  domain_name: <openstack-domain-name>
#  # tenant_name is Openstack tenant name
#  tenant_name: <openstack-tenant-name>
#  # username is Openstack username used to authenticate
#  username: <openstack-username>
#  # password is Openstack password used to authenticate
#  password: <openstack-password>`
