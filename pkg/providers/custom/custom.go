package custom

import (
	"context"
	"net/url"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/networkpolicy"
	"github.com/projectdiscovery/retryablehttp-go"
	sliceutil "github.com/projectdiscovery/utils/slice"
)

var Services = []string{"custom"}

type ProviderOptions struct {
	Id       string
	URLs     []string
	Headers  map[string]string
	Services schema.ServiceMap
}

const (
	urls         = "urls"
	headers      = "headers"
	providerName = "custom"
)

// Provider is a data provider for custom URLs
type Provider struct {
	client     *retryablehttp.Client
	id         string
	urlList    []string
	headerList map[string]string
	services   schema.ServiceMap
}

// New creates a new provider client for custom URLs
func New(block schema.OptionBlock) (*Provider, error) {
	options := &ProviderOptions{}
	if err := options.ParseOptionBlock(block); err != nil {
		return nil, err
	}

	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}

	services := make(schema.ServiceMap)
	for _, s := range Services {
		services[s] = struct{}{}
	}
	client := retryablehttp.NewClient(retryablehttp.DefaultOptionsSingle)
	return &Provider{client: client, id: options.Id, urlList: options.URLs, headerList: options.Headers, services: services}, nil
}

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.id
}

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.services.Keys()
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()
	serviceProvider := &serviceProvider{client: p.client, id: p.id, urlList: p.urlList, headerList: p.headerList}
	if services, err := serviceProvider.GetResource(ctx); err == nil {
		finalResources.Merge(services)
	}
	return finalResources, nil
}

func (p *ProviderOptions) ParseOptionBlock(block schema.OptionBlock) error {
	p.Id, _ = block.GetMetadata("id")

	supportedServicesMap := make(map[string]struct{})
	for _, s := range Services {
		supportedServicesMap[s] = struct{}{}
	}
	services := make(schema.ServiceMap)
	if ss, ok := block.GetMetadata("services"); ok {
		for _, s := range strings.Split(ss, ",") {
			if _, ok := supportedServicesMap[s]; ok {
				services[s] = struct{}{}
			}
		}
	}
	// if no services provided from -service flag, includes all services
	if len(services) == 0 {
		for _, s := range Services {
			services[s] = struct{}{}
		}
	}

	np, err := networkpolicy.New(networkpolicy.DefaultOptions)
	if err != nil {
		return err
	}

	if urlListStr, ok := block.GetMetadata(urls); ok {
		for _, urlStr := range sliceutil.Dedupe(strings.Split(urlListStr, ",")) {
			if parsedUrl, err := url.Parse(urlStr); err != nil || !np.Validate(parsedUrl.Hostname()) {
				gologger.Warning().Msgf("Invalid URL: %s\n", urlStr)
				continue
			}
			p.URLs = append(p.URLs, urlStr)
		}
	}

	if headerListStr, ok := block.GetMetadata(headers); ok {
		p.Headers = make(map[string]string)
		for _, header := range sliceutil.Dedupe(strings.Split(headerListStr, ",")) {
			if parts := strings.SplitN(header, ":", 2); len(parts) == 2 {
				key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				if key != "" && value != "" {
					p.Headers[key] = value
				}
			}
		}
	}
	return nil
}
