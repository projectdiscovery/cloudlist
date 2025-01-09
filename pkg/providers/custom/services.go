package custom

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/retryablehttp-go"
)

type serviceProvider struct {
	client     *retryablehttp.Client
	id         string
	urlList    []string
	headerList map[string]string
}

var re = regexp.MustCompile(`(?i)\b(?:https?://)?((?:[a-z0-9-]+\.)+)([a-z0-9-]+\.[a-z]{2,})(\.[a-z]{2,})?\b`)

// GetResource returns all the resources in the store for a provider.
func (d *serviceProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	for _, urlStr := range d.urlList {
		req, err := retryablehttp.NewRequest("GET", urlStr, nil)
		if err != nil {
			continue
		}

		for k, v := range d.headerList {
			req.Header.Set(k, v)
		}

		response, err := d.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			continue
		}

		subdomainSet := make(map[string]struct{})
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			line, _ = url.QueryUnescape(line)
			for _, subdomain := range re.FindAllString(line, -1) {
				subdomainSet[strings.ToLower(subdomain)] = struct{}{}
			}
		}

		for subdomain := range subdomainSet {
			list.Append(&schema.Resource{
				Provider: providerName,
				ID:       d.id,
				DNSName:  subdomain,
				Service:  providerName,
			})
		}
	}
	return list, nil
}
