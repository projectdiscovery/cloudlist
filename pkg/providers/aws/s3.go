package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// s3Provider is a provider for aws S3 API
type s3Provider struct {
	id      string
	s3      *s3.S3
	session *session.Session
}

func (d *s3Provider) name() string {
	return "s3"
}

// GetResource returns all the resources in the store for a provider.
func (d *s3Provider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()

	req := &s3.ListBucketsInput{}

	listBucketsOutput, err := d.s3.ListBuckets(req)
	if err != nil {
		return nil, errors.Wrap(err, "could not list s3 buckets")
	}
	for _, bucket := range listBucketsOutput.Buckets {
		endpointBuilder := &strings.Builder{}
		endpointBuilder.WriteString(aws.StringValue(bucket.Name))
		endpointBuilder.WriteString(".s3.amazonaws.com")

		list.Append(&schema.Resource{
			ID:       d.id,
			Public:   true,
			DNSName:  endpointBuilder.String(),
			Provider: providerName,
			Service:  d.name(),
		})
	}

	return list, nil
}
