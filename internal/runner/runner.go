package runner

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/projectdiscovery/cloudlist/pkg/inventory"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
)

// Runner is a client for running cloud provider asset enumeration
type Runner struct {
	config  schema.Options
	options *Options
}

// New creates a new runner instance based on configuration options
func New(options *Options) (*Runner, error) {

	if options.ProviderConfig == "" {
		options.ProviderConfig = defaultProviderConfigLocation
		gologger.Print().Msgf("Using default provider config: %s\n", options.ProviderConfig)
	}

	config, err := readProviderConfig(options.ProviderConfig)
	if err != nil {
		return nil, err
	}
	return &Runner{config: config, options: options}, nil
}

// Enumerate performs the cloudlist enumeration process
func (r *Runner) Enumerate() {
	finalConfig := schema.Options{}

	for _, item := range r.config {
		if item == nil {
			continue
		}
		if _, ok := item["id"]; !ok {
			item["id"] = ""
		}
		// Validate and only pass the correct items to input
		if len(r.options.Provider) != 0 || len(r.options.Id) != 0 {
			if len(r.options.Provider) != 0 && !Contains(r.options.Provider, item["provider"]) {
				continue
			}
			if len(r.options.Id) != 0 && !Contains(r.options.Id, item["id"]) {
				continue
			}
			finalConfig = append(finalConfig, item)
		} else {
			finalConfig = append(finalConfig, item)
		}
	}

	inventory, err := inventory.New(finalConfig)
	if err != nil {
		gologger.Fatal().Msgf("Could not create inventory: %s\n", err)
	}

	var output *os.File
	if r.options.Output != "" {
		outputFile, err := os.Create(r.options.Output)
		if err != nil {
			gologger.Fatal().Msgf("Could not create output file %s: %s\n", r.options.Output, err)
		}
		output = outputFile
	}

	builder := &bytes.Buffer{}
	for _, provider := range inventory.Providers {

		gologger.Info().Msgf("Listing assets from %s (%s) provider\n", provider.Name(), provider.ID())
		instances, err := provider.Resources(context.Background())
		if err != nil {
			gologger.Warning().Msgf("Could not get resources for provider %s %s: %s\n", provider.Name(), provider.ID(), err)
			continue
		}
		var hostsCount, ipCount int
		for _, instance := range instances.Items {
			builder.Reset()

			if r.options.JSON {
				data, err := jsoniter.Marshal(instance)
				if err != nil {
					gologger.Verbose().Msgf("ERR: Could not marshal json: %s\n", err)
				} else {
					builder.Write(data)
					builder.WriteString("\n")
					output.Write(builder.Bytes()) //nolint

					if instance.DNSName != "" {
						hostsCount++
					}
					if instance.PrivateIpv4 != "" {
						ipCount++
					}
					if instance.PublicIPv4 != "" {
						ipCount++
					}
					gologger.Silent().Msgf("%s", builder.String())
					builder.Reset()
				}
				continue
			}

			if r.options.Hosts {
				if instance.DNSName != "" {
					hostsCount++
					builder.WriteString(instance.DNSName)
					builder.WriteRune('\n')
					output.WriteString(builder.String()) //nolint
					builder.Reset()
					gologger.Silent().Msgf("%s", instance.DNSName)
				}
				continue
			}
			if r.options.IPAddress {
				if instance.PublicIPv4 != "" {
					ipCount++
					builder.WriteString(instance.PublicIPv4)
					builder.WriteRune('\n')
					output.WriteString(builder.String()) //nolint
					builder.Reset()
					gologger.Silent().Msgf("%s", instance.PublicIPv4)
				}
				if instance.PrivateIpv4 != "" && !r.options.ExcludePrivate {
					ipCount++
					builder.WriteString(instance.PrivateIpv4)
					builder.WriteRune('\n')
					output.WriteString(builder.String()) //nolint
					builder.Reset()
					gologger.Silent().Msgf("%s", instance.PrivateIpv4)
				}
				continue
			}

			if instance.DNSName != "" {
				hostsCount++
				builder.WriteString(instance.DNSName)
				builder.WriteRune('\n')
				output.WriteString(builder.String()) //nolint
				builder.Reset()
				gologger.Silent().Msgf("%s", instance.DNSName)
			}
			if instance.PublicIPv4 != "" {
				ipCount++
				builder.WriteString(instance.PublicIPv4)
				builder.WriteRune('\n')
				output.WriteString(builder.String()) //nolint
				builder.Reset()
				gologger.Silent().Msgf("%s", instance.PublicIPv4)
			}
			if instance.PrivateIpv4 != "" && !r.options.ExcludePrivate {
				ipCount++
				builder.WriteString(instance.PrivateIpv4)
				builder.WriteRune('\n')
				output.WriteString(builder.String()) //nolint
				builder.Reset()
				gologger.Silent().Msgf("%s", instance.PrivateIpv4)
			}
		}
		logBuilder := &strings.Builder{}
		if hostsCount != 0 {
			logBuilder.WriteString(strconv.Itoa(hostsCount))
			logBuilder.WriteString(" Hosts")
		}
		if ipCount != 0 {
			if hostsCount != 0 {
				logBuilder.WriteString(" and ")
			}
			logBuilder.WriteString(strconv.Itoa(ipCount))
			logBuilder.WriteString(" IP Addresses")
		}
		if hostsCount == 0 && ipCount == 0 {
			gologger.Warning().Msgf("No results found for %s (%s)\n", provider.Name(), provider.ID())
		} else {
			gologger.Info().Msgf("Found %s for %s (%s)\n", logBuilder.String(), provider.Name(), provider.ID())
		}
	}
}

func Contains(s []string, e string) bool {
	for _, a := range s {
		if strings.EqualFold(a, e) {
			return true
		}
	}
	return false
}
