package runner

import (
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/fileutil"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/levels"
	"gopkg.in/yaml.v2"
)

// Options contains the configuration options for cloudlist.
type Options struct {
	JSON           bool   // JSON returns JSON output
	Silent         bool   // Silent Display results only
	Version        bool   // Version returns the version of the tool.
	Verbose        bool   // Verbose prints verbose output.
	Hosts          bool   // Hosts specifies to fetch only DNS Names
	IPAddress      bool   // IPAddress specifes to fetch only IP Addresses
	Config         string // Config is the location of the config file.
	Output         string // Output is the file to write found results too.
	ExcludePrivate bool   // ExcludePrivate excludes private IPs from results
	Provider       string // Provider specifies what providers to fetch assets for.
}

var defaultConfigLocation = path.Join(userHomeDir(), "/.config/cloudlist/config.yaml")

// ParseOptions parses the command line flags provided by a user
func ParseOptions() *Options {
	options := &Options{}

	flag.BoolVar(&options.JSON, "json", false, "Show json output")
	flag.BoolVar(&options.Silent, "silent", false, "Show only results in output")
	flag.BoolVar(&options.Version, "version", false, "Show version of cloudlist")
	flag.BoolVar(&options.Verbose, "v", false, "Show Verbose output")
	flag.BoolVar(&options.Hosts, "host", false, "Show only hosts in output")
	flag.BoolVar(&options.IPAddress, "ip", false, "Show only IP addresses in output")
	flag.StringVar(&options.Config, "config", defaultConfigLocation, "Configuration file to use for enumeration")
	flag.StringVar(&options.Output, "o", "", "File to write output to (optional)")
	flag.StringVar(&options.Provider, "provider", "", "Provider to fetch assets from (optional)")
	flag.BoolVar(&options.ExcludePrivate, "exclude-private", false, "Exclude private IP addresses from output")
	flag.Parse()

	options.configureOutput()
	showBanner()

	if options.Version {
		gologger.Info().Msgf("Current Version: %s\n", Version)
		os.Exit(0)
	}
	checkAndCreateConfigFile(options)
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

// readConfig reads the config file from the options
func readConfig(configFile string) (schema.Options, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := schema.Options{}
	if err := yaml.NewDecoder(file).Decode(&config); err != nil {
		if err == io.EOF {
			return nil, errors.New("invalid configuration file provided")
		}
		return nil, err
	}
	return config, nil
}

// checkAndCreateConfigFile checks if a config file exists,
// if not creates a default.
func checkAndCreateConfigFile(options *Options) {
	if options.Config == defaultConfigLocation {
		err := os.MkdirAll(path.Dir(options.Config), os.ModePerm)
		if err != nil {
			gologger.Warning().Msgf("Could not create default config file: %s\n", err)
		}
		if !fileutil.FileExists(defaultConfigLocation) {
			if writeErr := ioutil.WriteFile(defaultConfigLocation, []byte(defaultConfigFile), os.ModePerm); writeErr != nil {
				gologger.Warning().Msgf("Could not write default output to %s: %s\n", defaultConfigLocation, writeErr)
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

const defaultConfigFile = `# Configuration file for cloudlist enumeration agent
#- # provider is the name of the provider
#  provider: do
#  # profile is the name of the provider profile
#  profile: xxxx
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
#  # profile is the name of the provider profile
#  profile: staging
#  # aws_access_key is the access key for AWS account
#  aws_access_key: AKIAXXXXXXXXXXXXXX
#  # aws_secret_key is the secret key for AWS account
#  aws_secret_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

#- # provider is the name of the provider
#  provider: cloudflare
#  # email is the email for cloudflare
#  email: user@domain.com
#  # api_key is the api_key for cloudflare
#  api_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

#- # provider is the name of the provider
#  provider: heroku
#  # profile is the name of the provider profile
#  profile: staging
#  # heroku_api_token is the api key for Heroku account
#  heroku_api_token: xxxxxxxxxxxxxxxxxxxx

#- # provider is the name of the provider
#  provider: linode
#  # profile is the name of the provider profile
#  profile: staging
#  # linode_personal_access_token is the personal access token for linode account
#  linode_personal_access_token: XXXXXXXXXXXXXXXXXXXXXXXX

#- # provider is the name of the provider
#  provider: fastly
#  # profile is the name of the provider profile
#  profile: staging
#  # fastly_api_key is the personal API token for fastly account
#  fastly_api_key: XX-XXXXXXXXXXXXXXXXXXXXXX-

#- # provider is the name of the provider
#  provider: alibaba
#  # profile is the name of the provider profile
#  profile: staging
#  # alibaba_region_id is the region id of the resources
#  alibaba_region_id: ap-XXXXXXX
#  # alibaba_access_key is the access key ID for alibaba cloud account
#  alibaba_access_key: XXXXXXXXXXXXXXXXXXXX
#  # alibaba_access_key_secret is the secret access key for alibaba cloud account
#  alibaba_access_key_secret: XXXXXXXXXXXXXXXX

# - # provider is the name of the provider
#  provider: namecheap
#  # profile is the name of the provider profile
#  profile: staging
#  # namecheap_api_key is the api key for namecheap account
#  namecheap_api_key: XXXXXXXXXXXXXXXXXXXXXXX
#  # namecheap_user_name is the username of the namecheap account
#  namecheap_user_name: XXXXXXX`
