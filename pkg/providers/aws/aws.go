package aws

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// Provider is a data provider for aws API
type Provider struct {
	profile       string
	ec2Client     *ec2.EC2
	route53Client *route53.Route53
	regions       *ec2.DescribeRegionsOutput
	session       *session.Session
}

// New creates a new provider client for aws API
func New(options schema.OptionBlock) (*Provider, error) {
	accessKey, ok := options.GetMetadata(apiAccessKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	accessToken, ok := options.GetMetadata(apiSecretKey)
	if !ok {
		return nil, &schema.ErrNoSuchKey{Name: apiSecretKey}
	}
	token, _ := options.GetMetadata(sessionToken)
	profile, _ := options.GetMetadata("profile")

	config := aws.NewConfig()
	config.WithRegion("us-east-1")
	config.WithCredentials(credentials.NewStaticCredentials(accessKey, accessToken, token))

	session, err := session.NewSession(config)
	if err != nil {
		return nil, errors.Wrap(err, "could not extablish a session")
	}

	ec2Client := ec2.New(session)
	route53Client := route53.New(session)

	regions, err := ec2Client.DescribeRegions(&ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, errors.Wrap(err, "could not get list of regions")
	}
	return &Provider{ec2Client: ec2Client, profile: profile, regions: regions, route53Client: route53Client, session: session}, nil
}

const apiAccessKey = "aws_access_key"
const apiSecretKey = "aws_secret_key"
const sessionToken = "aws_session_token"
const providerName = "aws"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ProfileName returns the name of the provider profile
func (p *Provider) ProfileName() string {
	return p.profile
}

// Resources returns the provider for an resource deployment source.
func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	ec2provider := &instanceProvider{ec2Client: p.ec2Client, profile: p.profile, session: p.session, regions: p.regions}
	list, err := ec2provider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	route53Provider := &route53Provider{route53: p.route53Client, profile: p.profile, session: p.session}
	zones, err := route53Provider.GetResource(ctx)
	if err != nil {
		return nil, err
	}
	finalList := schema.NewResources()
	finalList.Merge(list)
	finalList.Merge(zones)
	return finalList, nil
}
