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
	config, err := readConfig(options.Config)
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
		if _, ok := item["profile"]; !ok {
			item["profile"] = "none"
		}
		// Validate and only pass the correct items to input
		if r.options.Provider != "" {
			if item["provider"] != r.options.Provider {
				gologger.Verbose().Msgf("Skipping provider %s due to command line\n", "WRN", item["provider"])
				continue
			} else {
				finalConfig = append(finalConfig, item)
			}
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
		if r.options.Provider != "" {
			if provider.Name() != r.options.Provider {
				continue
			}
		}

		gologger.Info().Msgf("Listing assets from %s (%s) provider\n", provider.Name(), provider.ProfileName())
		instances, err := provider.Resources(context.Background())
		if err != nil {
			gologger.Warning().Msgf("Could not get resources for provider %s %s: %s\n", provider.Name(), provider.ProfileName(), err)
			continue
		}
		var hostsCount, ipCount int
		for _, instance := range instances.Items {
			builder.Reset()

			if r.options.JSON {
				data, err := jsoniter.Marshal(instance)
				if err != nil {
					gologger.Verbose().Msgf("Could not marshal json: %s\n", "ERR", err)
				} else {
					builder.Write(data)
					builder.WriteString("\n")
					output.Write(builder.Bytes())

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
					output.WriteString(builder.String())
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
					output.WriteString(builder.String())
					builder.Reset()
					gologger.Silent().Msgf("%s", instance.PublicIPv4)
				}
				if instance.PrivateIpv4 != "" {
					ipCount++
					builder.WriteString(instance.PrivateIpv4)
					builder.WriteRune('\n')
					output.WriteString(builder.String())
					builder.Reset()
					gologger.Silent().Msgf("%s", instance.PrivateIpv4)
				}
				continue
			}

			if instance.DNSName != "" {
				hostsCount++
				builder.WriteString(instance.DNSName)
				builder.WriteRune('\n')
				output.WriteString(builder.String())
				builder.Reset()
				gologger.Silent().Msgf("%s", instance.DNSName)
			}
			if instance.PublicIPv4 != "" {
				ipCount++
				builder.WriteString(instance.PublicIPv4)
				builder.WriteRune('\n')
				output.WriteString(builder.String())
				builder.Reset()
				gologger.Silent().Msgf("%s", instance.PublicIPv4)
			}
			if instance.PrivateIpv4 != "" {
				ipCount++
				builder.WriteString(instance.PrivateIpv4)
				builder.WriteRune('\n')
				output.WriteString(builder.String())
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
			gologger.Warning().Msgf("No results found for %s (%s)\n", provider.Name(), provider.ProfileName())
		} else {
			gologger.Info().Msgf("Found %s for %s (%s)\n", logBuilder.String(), provider.Name(), provider.ProfileName())
		}
	}
}
