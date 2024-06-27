package namecheap

import (
	"context"

	"github.com/namecheap/go-namecheap-sdk/v2/namecheap"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// domainProvider is an domain provider for NameCheap API
type domainProvider struct {
	id     string
	client *namecheap.Client
}

func (d *domainProvider) name() string {
	return "domain"
}

// GetResource returns all the applications for the NameCheap provider.
func (d *domainProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	page := 1
	pageSize := 100

	for {
		domainList, err := d.client.Domains.GetList(&namecheap.DomainsGetListArgs{Page: &page, PageSize: &pageSize})
		if err != nil {
			return nil, err
		}

		if domainList.Domains == nil {
			break
		}
		for _, domain := range *domainList.Domains {

			list.Append(&schema.Resource{
				Provider: providerName,
				DNSName:  *domain.Name,
				ID:       d.id,
				Service:  d.name(),
			})
		}
		if *domainList.Paging.TotalItems <= (*domainList.Paging.PageSize * *domainList.Paging.CurrentPage) {
			break
		}
		page++
	}

	return list, nil
}
