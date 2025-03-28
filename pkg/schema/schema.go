package schema

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/projectdiscovery/cloudlist/pkg/schema/validate"
	mapsutil "github.com/projectdiscovery/utils/maps"
)

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
	// Services returns the services provided by the Provider.
	// If no services set, it will return all the supported services.
	Services() []string
}

// VerifiableProvider is a Provider that can be verified
// with additional optional verification mechanism.
type VerifiableProvider interface {
	Provider
	// Verify checks if the provider is valid
	Verify(ctx context.Context) error
}

// Resources is a container of multiple resource returned from providers
type Resources struct {
	Items        []*Resource
	deduplicator *ResourceDeduplicator
}

// NewResources creates a new resources structure
func NewResources() *Resources {
	return &Resources{
		Items:        make([]*Resource, 0),
		deduplicator: NewResourceDeduplicator(),
	}
}

var (
	validator *validate.Validator
)

func init() {
	// Create validator
	var err error
	validator, err = validate.NewValidator()
	if err != nil {
		panic(fmt.Sprintf("Could not create validator: %s\n", err))
	}
}

// appendResourceWithTypeAndMeta appends a resource with a type and metadata
func (r *Resources) appendResourceWithTypeAndMeta(resourceType validate.ResourceType, item, id, provider, service string) {
	resource := &Resource{
		Provider: provider,
		ID:       id,
		Service:  service,
	}
	switch resourceType {
	case validate.DNSName:
		resource.Public = true
		resource.DNSName = item
	case validate.PublicIPv4:
		resource.Public = true
		resource.PublicIPv4 = item
	case validate.PublicIPv6:
		resource.Public = true
		resource.PublicIPv6 = item
	case validate.PrivateIPv4:
		resource.PrivateIpv4 = item
	case validate.PrivateIPv6:
		resource.PrivateIpv6 = item
	default:
		return
	}
	r.Items = append(r.Items, resource)
}

// appendResource appends a resource to the resources list
func (r *Resources) appendResource(resource *Resource) {
	if resource.DNSName != "" && !r.deduplicator.Contains(resource.DNSName) {
		resourceType := validator.Identify(resource.DNSName)
		r.appendResourceWithTypeAndMeta(resourceType, resource.DNSName, resource.ID, resource.Provider, resource.Service)
		r.deduplicator.Add(resource.DNSName)
	}

	if resource.PublicIPv4 != "" && !r.deduplicator.Contains(resource.PublicIPv4) {
		resourceType := validator.Identify(resource.PublicIPv4)
		r.appendResourceWithTypeAndMeta(resourceType, resource.PublicIPv4, resource.ID, resource.Provider, resource.Service)
		r.deduplicator.Add(resource.PublicIPv4)
	}

	if resource.PublicIPv6 != "" && !r.deduplicator.Contains(resource.PublicIPv6) {
		resourceType := validator.Identify(resource.PublicIPv6)
		r.appendResourceWithTypeAndMeta(resourceType, resource.PublicIPv6, resource.ID, resource.Provider, resource.Service)
		r.deduplicator.Add(resource.PublicIPv6)
	}

	if resource.PrivateIpv4 != "" && !r.deduplicator.Contains(resource.PrivateIpv4) {
		resourceType := validator.Identify(resource.PrivateIpv4)
		r.appendResourceWithTypeAndMeta(resourceType, resource.PrivateIpv4, resource.ID, resource.Provider, resource.Service)
		r.deduplicator.Add(resource.PrivateIpv4)
	}

	if resource.PrivateIpv6 != "" && !r.deduplicator.Contains(resource.PrivateIpv6) {
		resourceType := validator.Identify(resource.PrivateIpv6)
		r.appendResourceWithTypeAndMeta(resourceType, resource.PrivateIpv6, resource.ID, resource.Provider, resource.Service)
		r.deduplicator.Add(resource.PrivateIpv6)
	}
}

// Append appends a single resource to the resource list
func (r *Resources) Append(resource *Resource) {
	r.appendResource(resource)
}

// Merge merges a list of resources into the main list
func (r *Resources) Merge(resources *Resources) {
	if resources == nil {
		return
	}
	for _, item := range resources.Items {
		r.appendResource(item)
	}
}

// Resource is a cloud resource belonging to the organization
type Resource struct {
	// Public specifies whether the asset is public facing or private
	Public bool `json:"public"`
	// Provider is the name of provider for instance
	Provider string `json:"provider"`
	// Service is the name of the service under the provider
	Service string `json:"service,omitempty"`
	// ID is the id name of the resource provider
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

// ErrNoSuchKey means no such key exists in metadata.
type ErrNoSuchKey struct {
	Name string
}

// Error returns the value of the metadata key
func (e *ErrNoSuchKey) Error() string {
	return fmt.Sprintf("no such key: %s", e.Name)
}

// Options contains configuration options for a provider
type Options []OptionBlock

// GetServiceNames returns the services from the options
func (o Options) GetServiceNames() []string {
	services := make([]string, 0)
	for _, option := range o {
		if serviceNameList, ok := option["services"]; ok {
			for _, serviceName := range strings.Split(serviceNameList, ",") {
				trimmedServiceName := strings.TrimSpace(serviceName)
				if trimmedServiceName != "" {
					services = append(services, trimmedServiceName)
				}
			}
		}
	}
	return services
}

// OptionBlock is a single option on which operation is possible
type OptionBlock map[string]string

func (ob *OptionBlock) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Unmarshal into a raw map
	var rawMap map[string]interface{}
	if err := unmarshal(&rawMap); err != nil {
		return err
	}
	// Initialize the OptionBlock
	*ob = make(OptionBlock)
	// Convert raw map to OptionBlock and handle special cases
	for key, value := range rawMap {
		switch key {
		case "account_ids", "urls", "services":
			if valueArr, ok := value.([]interface{}); ok {
				var strArr []string
				for _, v := range valueArr {
					switch v := v.(type) {
					case string:
						strArr = append(strArr, v)
					case int:
						strArr = append(strArr, fmt.Sprint(v))
					default:
						return fmt.Errorf("unsupported type %T in account_ids", v)
					}
				}
				(*ob)[key] = strings.Join(strArr, ",")
			}
		case "headers":
			if valueMap, ok := value.(map[interface{}]interface{}); ok {
				var strArr []string
				for k, v := range valueMap {
					strArr = append(strArr, fmt.Sprintf("%s: %s", k, v))
				}
				(*ob)[key] = strings.Join(strArr, ",")
			}
		default:
			(*ob)[key] = fmt.Sprint(value)
		}
	}
	return nil
}

// GetMetadata returns the value for a key if it exists.
func (o OptionBlock) GetMetadata(key string) (string, bool) {
	data, ok := o[key]
	if !ok || data == "" {
		return "", false
	}
	// if data starts with $, treat it as an env var
	if data[0] == '$' {
		envData := os.Getenv(data[1:])
		if envData != "" {
			return envData, true
		}
	}
	return data, true
}

type ServiceMap map[string]struct{}

func (s ServiceMap) Has(service string) bool {
	_, ok := s[service]
	return ok
}

func (s ServiceMap) Keys() []string {
	return mapsutil.GetKeys(s)
}

// ResourceDeduplicator provides thread-safe deduplication for resources
type ResourceDeduplicator struct {
	items *sync.Map
}

// NewResourceDeduplicator creates a new resource deduplicator
func NewResourceDeduplicator() *ResourceDeduplicator {
	return &ResourceDeduplicator{
		items: &sync.Map{},
	}
}

// Contains checks if a value exists in the deduplicator
func (d *ResourceDeduplicator) Contains(value string) bool {
	if value == "" {
		return false
	}
	_, exists := d.items.Load(value)
	return exists
}

// Add adds a value to the deduplicator if it doesn't exist
// Returns true if the value was added (was unique)
func (d *ResourceDeduplicator) Add(value string) bool {
	if value == "" {
		return false
	}
	_, loaded := d.items.LoadOrStore(value, struct{}{})
	return !loaded
}

// ProcessResource adds a resource's values to deduplication and returns if any were new
func (d *ResourceDeduplicator) ProcessResource(resource *Resource) bool {
	added := false

	if resource.DNSName != "" && !d.Contains(resource.DNSName) {
		d.Add(resource.DNSName)
		added = true
	}

	if resource.PublicIPv4 != "" && !d.Contains(resource.PublicIPv4) {
		d.Add(resource.PublicIPv4)
		added = true
	}

	if resource.PublicIPv6 != "" && !d.Contains(resource.PublicIPv6) {
		d.Add(resource.PublicIPv6)
		added = true
	}

	if resource.PrivateIpv4 != "" && !d.Contains(resource.PrivateIpv4) {
		d.Add(resource.PrivateIpv4)
		added = true
	}

	if resource.PrivateIpv6 != "" && !d.Contains(resource.PrivateIpv6) {
		d.Add(resource.PrivateIpv6)
		added = true
	}

	return added
}
