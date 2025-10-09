package validate

import (
	"net"
	"regexp"
	"strings"
)

// ipv4PrivateRanges contains the IP ranges marked private in IPv4.
var ipv4PrivateRanges = []string{
	"0.0.0.0/8",       // Current network (only valid as source address)
	"10.0.0.0/8",      // Private network
	"100.64.0.0/10",   // Shared Address Space
	"127.0.0.0/8",     // Loopback
	"169.254.0.0/16",  // Link-local (Also many cloud providers Metadata endpoint)
	"172.16.0.0/12",   // Private network
	"192.0.0.0/24",    // IETF Protocol Assignments
	"192.0.2.0/24",    // TEST-NET-1, documentation and examples
	"192.88.99.0/24",  // IPv6 to IPv4 relay (includes 2002::/16)
	"192.168.0.0/16",  // Private network
	"198.18.0.0/15",   // Network benchmark tests
	"198.51.100.0/24", // TEST-NET-2, documentation and examples
	"203.0.113.0/24",  // TEST-NET-3, documentation and examples
	"224.0.0.0/4",     // IP multicast (former Class D network)
	"240.0.0.0/4",     // Reserved (former Class E network)
}

// ipv6PrivateRanges contains the IP ranges marked private in IPv6.
var ipv6PrivateRanges = []string{
	"::1/128",       // Loopback
	"64:ff9b::/96",  // IPv4/IPv6 translation (RFC 6052)
	"100::/64",      // Discard prefix (RFC 6666)
	"2001::/32",     // Teredo tunneling
	"2001:10::/28",  // Deprecated (previously ORCHID)
	"2001:20::/28",  // ORCHIDv2
	"2001:db8::/32", // Addresses used in documentation and example source code
	"2002::/16",     // 6to4
	"fc00::/7",      // Unique local address
	"fe80::/10",     // Link-local address
	"ff00::/8",      // Multicast
}

// Validator contains a validator for identifying type of resource
type Validator struct {
	ipv4      []*net.IPNet
	ipv6      []*net.IPNet
	rxDNSName *regexp.Regexp
}

// NewValidator creates a validator for identifying type of resource
func NewValidator() (*Validator, error) {
	validator := &Validator{}
	// regex from: https://github.com/asaskevich/govalidator
	validator.rxDNSName = regexp.MustCompile(`^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62})*[\._]?$`)

	err := validator.appendIPv4Ranges(ipv4PrivateRanges)
	if err != nil {
		return nil, err
	}
	err = validator.appendIPv6Ranges(ipv6PrivateRanges)
	if err != nil {
		return nil, err
	}
	return validator, nil
}

// ResourceType is the type of the resource returned
type ResourceType int

// A list of supported resource types
const (
	DNSName ResourceType = iota + 1
	PublicIPv4
	PublicIPv6
	PrivateIPv4
	PrivateIPv6
	None
)

// Identify returns the resourcetype for an item
func (v *Validator) Identify(item string) ResourceType {
	host, port, err := net.SplitHostPort(item)
	if err == nil && port != "" {
		item = host
	}
	parsed := net.ParseIP(item)
	if v.isDNSName(item, parsed) {
		return DNSName
	}
	if parsed == nil {
		return None
	}
	if strings.Contains(item, ":") {
		// Check ipv6 private address list
		if v.containsIPv6(parsed) {
			return PrivateIPv6
		}
		return PublicIPv6
	}
	// Check ipv4 private address list
	if v.containsIPv4(parsed) {
		return PrivateIPv4
	}
	return PublicIPv4
}

// isDNSName will validate the given string as a DNS name
func (v *Validator) isDNSName(str string, parsed net.IP) bool {
	if str == "" || len(strings.ReplaceAll(str, ".", "")) > 255 {
		// constraints already violated
		return false
	}
	return !isIP(parsed) && v.rxDNSName.MatchString(str)
}

// isIP checks if a string is either IP version 4 or 6. Alias for `net.ParseIP`
func isIP(parsed net.IP) bool {
	return parsed != nil
}

// appendIPv4Ranges adds a list of IPv4 Ranges to the blacklist.
func (v *Validator) appendIPv4Ranges(ranges []string) error {
	for _, ip := range ranges {
		_, rangeNet, err := net.ParseCIDR(ip)
		if err != nil {
			return err
		}
		v.ipv4 = append(v.ipv4, rangeNet)
	}
	return nil
}

// appendIPv6Ranges adds a list of IPv6 Ranges to the blacklist.
func (v *Validator) appendIPv6Ranges(ranges []string) error {
	for _, ip := range ranges {
		_, rangeNet, err := net.ParseCIDR(ip)
		if err != nil {
			return err
		}
		v.ipv6 = append(v.ipv6, rangeNet)
	}
	return nil
}

// containsIPv4 checks whether a given IP address exists in the blacklisted IPv4 ranges.
func (v *Validator) containsIPv4(IP net.IP) bool {
	for _, net := range v.ipv4 {
		if net.Contains(IP) {
			return true
		}
	}
	return false
}

// CcntainsIPv6 checks whether a given IP address exists in the blacklisted IPv6 ranges.
func (v *Validator) containsIPv6(IP net.IP) bool {
	for _, net := range v.ipv6 {
		if net.Contains(IP) {
			return true
		}
	}
	return false
}
