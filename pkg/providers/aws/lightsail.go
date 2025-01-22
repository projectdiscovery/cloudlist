package aws

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// lightsailProvider is an instance provider for AWS Lightsail API
type lightsailProvider struct {
	options  ProviderOptions
	lsClient *lightsail.Lightsail
	session  *session.Session
	regions  []*lightsail.Region
}

func (l *lightsailProvider) name() string {
	return "lightsail"
}

// GetResource returns all the resources in the store for a provider.
func (l *lightsailProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range l.regions {
		for _, lsClient := range l.getLightsailClients(region.Name) {
			wg.Add(1)

			go func(client *lightsail.Lightsail) {
				defer wg.Done()

				if resources, err := l.listListsailResources(client); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(lsClient)
		}
	}
	wg.Wait()
	return list, nil
}

func (l *lightsailProvider) listListsailResources(lsClient *lightsail.Lightsail) (*schema.Resources, error) {
	list := schema.NewResources()
	req := &lightsail.GetInstancesInput{}
	for {
		resp, err := lsClient.GetInstances(req)
		if err != nil {
			return nil, err
		}

		for _, instance := range resp.Instances {
			privateIPv4 := aws.StringValue(instance.PrivateIpAddress)
			publicIPv4 := aws.StringValue(instance.PublicIpAddress)
			resource := &schema.Resource{
				ID:          l.options.Id,
				Provider:    providerName,
				PrivateIpv4: privateIPv4,
				PublicIPv4:  publicIPv4,
				Public:      publicIPv4 != "",
				Service:     l.name(),
			}

			if len(instance.Ipv6Addresses) > 0 {
				resource.PublicIPv6 = aws.StringValue(instance.Ipv6Addresses[0])
			}

			list.Append(resource)
		}
		if aws.StringValue(resp.NextPageToken) == "" {
			break
		}
		req.PageToken = resp.NextPageToken
	}
	return list, nil
}

func (l *lightsailProvider) getLightsailClients(region *string) []*lightsail.Lightsail {
	endpoint := fmt.Sprintf("https://lightsail.%s.amazonaws.com", aws.StringValue(region))
	lightsailClients := make([]*lightsail.Lightsail, 0)

	lightsailClient := lightsail.New(
		l.session,
		aws.NewConfig().WithEndpoint(endpoint),
		aws.NewConfig().WithRegion(aws.StringValue(region)),
	)
	lightsailClients = append(lightsailClients, lightsailClient)

	if l.options.AssumeRoleName == "" || len(l.options.AccountIds) < 1 {
		return lightsailClients
	}

	for _, accountId := range l.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, l.options.AssumeRoleName)
		creds := stscreds.NewCredentials(l.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}

		lightsailClients = append(lightsailClients, lightsail.New(assumeSession))
	}
	return lightsailClients
}
