## DESIGN

Cloudlist is an Asset Discovery tool designed to gather information from multiple providers (using their specific APIs) related to Target Hostnames or IP Addresses registered in those providers. The providers here can be managed DNS solutions like cloudflare, AWS Route53, etc or Compute As a Service Providers like AWS EC2, Digitalocean, GCP, etc.

It is designed in such a way that adding providers is a breeze. There a few main types of the design that we need to describe first.

https://github.com/projectdiscovery/cloudlist/tree/main/pkg -

### Providers
Each Provider implements the below described `schema.Provider` interface. The core logic of the provider is contained in `Resources()` function, which returns a list of resources that the provider currently has.

```go
// Provider is an interface implemented by any cloud service provider.
//
// It provides the bare minimum of methods to allow complete overview of user
// data.
type Provider interface {
	// Name returns the name of the provider
	Name() string
	// ID returns the name of the provider id
	ID() string
	// Resources returns the provider for an resource deployment source.
	Resources(ctx context.Context) (*Resources, error)
}
```

Each Provider takes an `schema.OptionBlock` slice for initialization, which in turn is a key-value map of strings. A `GetMetadata` function is also provided to reduce some typing. Providers return an `ErrNoSuchKey` if an option they expected in `schema.OptionBlock` is not avilable.

```go
// Options contains configuration options for a provider
type Options []OptionBlock

// OptionBlock is a single option on which operation is possible
type OptionBlock map[string]string

// GetMetadata returns the value for a key if it exists.
func (o OptionBlock) GetMetadata(key string) (string, bool) {
	data, ok := o[key]
	if !ok || data == "" {
		return "", false
	}
	return data, true
}
```

The `nameToProvider` function is reponsible for handling creation and initialization of the providers at https://github.com/projectdiscovery/cloudlist/blob/main/pkg/inventory/inventory.go#L40. Just add a case for your new provider and return it by calling `<provider>.New(block)`. 

```go
switch value {
	case "aws":
		return aws.New(block)
	case "do":
		return digitalocean.New(block)
	case "gcp":
		return gcp.New(block)
	case "scw":
		return scaleway.New(block)
	default:
		return nil, fmt.Errorf("invalid provider name found: %s", value)
	}
```

### Resource

A resource is a single unit in cloud belonging to an Organization. Some metadata is provided, like whether is the asset public facing or private, provider, id name, as well as any IP addresses and DNS Names (Either among IP or DNS must always be provided).

Providers return `schema.Resource` structure that contains an array of resources and provides some convenience wrappers on top of the array like `Append` and `Merge`. These can be used during the resource collection phase to minimize boilerplate.

```go
// Resource is a cloud resource belonging to the organization
type Resource struct {
	// Public specifies whether the asset is public facing or private
	Public bool `json:"public"`
	// Provider is the name of provider for instance
	Provider string `json:"provider"`
	// ID is the id the resource provider
	ID string `json:"id,omitempty"`
	// PublicIPv4 is the public ipv4 address of the instance.
	PublicIPv4 string `json:"public_ipv4,omitempty"`
	// PublicIPv6 is the public ipv6 address of the instance.
	PublicIPv6 string `json:"public_ipv6,omitempty"`
	// PrivateIpv4 is the private ipv4 address of the instance
	PrivateIpv4 string `json:"private_ipv4,omitempty"`
	// PrivateIpv6 is the private ipv6 address of the instance
	PrivateIpv6 string `json:"private_ipv6,omitempty"`
	// DNSName is the DNS name of the resource
	DNSName string `json:"dns_name,omitempty"`
}
```

### Adding a new provider

Steps - 

1. Add the code for the provider in pkg/providers.
2. Add the provider to nameToProvider function as a case with intiialization function for the provider.
3. Test the provider integration.
4. Add documentation on how to use the integration (configuration it accepts, steps to generate) to https://github.com/projectdiscovery/cloudlist/blob/main/PROVIDERS.md - Connect to preview .

References - 

1. https://github.com/projectdiscovery/cloudlist/tree/main/pkg/providers/aws - AWS Route53 And EC2 provider. Sub-structures are used for both resource types which are called by a Main resources() function. 
2. https://github.com/projectdiscovery/cloudlist/tree/main/pkg/providers/digitalocean - Digitalocean Instances integration. Probably the simplest one.
3. https://github.com/projectdiscovery/cloudlist/tree/main/pkg/providers/gcp - Google Cloud DNS integration. Demonstrates a complex JSON config in use.
