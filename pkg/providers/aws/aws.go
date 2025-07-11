package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	sliceutil "github.com/projectdiscovery/utils/slice"
)

var Services = []string{"ec2", "instance", "route53", "s3", "ecs", "eks", "lambda", "apigateway", "alb", "elb", "lightsail", "cloudfront"}

type ProviderOptions struct {
	Id                    string
	AccessKey             string
	SecretKey             string
	Token                 string
	AssumeRoleArn         string
	AssumeRoleSessionName string
	ExternalId            string
	AssumeRoleName        string
	AccountIds            []string
	Services              schema.ServiceMap
}

func (p *ProviderOptions) ParseOptionBlock(block schema.OptionBlock) error {
	p.Id, _ = block.GetMetadata("id")
	accessKey, ok := block.GetMetadata(apiAccessKey)
	if !ok {
		return &schema.ErrNoSuchKey{Name: apiAccessKey}
	}
	accessToken, ok := block.GetMetadata(apiSecretKey)
	if !ok {
		return &schema.ErrNoSuchKey{Name: apiSecretKey}
	}
	p.Token, _ = block.GetMetadata(sessionToken)
	p.AccessKey = accessKey
	p.SecretKey = accessToken

	if assumeRoleArn, ok := block.GetMetadata(assumeRoleArn); ok {
		p.AssumeRoleArn = assumeRoleArn
	}
	if assumeRoleSessionName, ok := block.GetMetadata(assumeRoleSessionName); ok {
		p.AssumeRoleSessionName = assumeRoleSessionName
	}

	if externalId, ok := block.GetMetadata(externalId); ok {
		p.ExternalId = externalId
	}

	if assumeRoleName, ok := block.GetMetadata(assumeRoleName); ok {
		p.AssumeRoleName = assumeRoleName
	}

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
	p.Services = services

	if accountIds, ok := block.GetMetadata(accountIds); ok {
		p.AccountIds = sliceutil.Dedupe(strings.Split(accountIds, ","))
	}
	return nil
}

// Provider is a data provider for aws API
type Provider struct {
	options          *ProviderOptions
	ec2Client        *ec2.EC2
	route53Client    *route53.Route53
	s3Client         *s3.S3
	ecsClient        *ecs.ECS
	eksClient        *eks.EKS
	lambdaClient     *lambda.Lambda
	apiGateway       *apigateway.APIGateway
	albClient        *elbv2.ELBV2
	elbClient        *elb.ELB
	lightsailClient  *lightsail.Lightsail
	cloudFrontClient *cloudfront.CloudFront
	regions          *ec2.DescribeRegionsOutput
	session          *session.Session
}

// New creates a new provider client for aws API
func New(block schema.OptionBlock) (*Provider, error) {
	options := &ProviderOptions{}
	if err := options.ParseOptionBlock(block); err != nil {
		return nil, err
	}

	provider := &Provider{options: options}
	config := aws.NewConfig()
	config.WithRegion("us-east-1")
	config.WithCredentials(credentials.NewStaticCredentials(options.AccessKey, options.SecretKey, options.Token))

	var sess *session.Session
	var err error

	// Handle role assumption for assume_role_arn case
	if options.AssumeRoleArn != "" {
		stsSession, err := session.NewSession(config)
		if err != nil {
			return nil, errors.Wrap(err, "could not establish session with AWS config")
		}

		stsClient := sts.New(stsSession)
		roleInput := &sts.AssumeRoleInput{
			RoleArn: aws.String(options.AssumeRoleArn),
		}

		// Only set optional fields if they are provided
		if options.AssumeRoleSessionName != "" {
			roleInput.RoleSessionName = aws.String(options.AssumeRoleSessionName)
		} else {
			roleInput.RoleSessionName = aws.String("cloudlist-session")
		}

		if options.ExternalId != "" {
			roleInput.ExternalId = aws.String(options.ExternalId)
		}

		assumeRoleOutput, err := stsClient.AssumeRole(roleInput)
		if err != nil {
			return nil, errors.Wrap(err, "failed to assume role")
		}

		assumedCredentials := assumeRoleOutput.Credentials
		sess, err = session.NewSession(&aws.Config{
			Credentials: credentials.NewStaticCredentials(
				*assumedCredentials.AccessKeyId,
				*assumedCredentials.SecretAccessKey,
				*assumedCredentials.SessionToken,
			),
			Region: config.Region,
		})
		if err != nil {
			return nil, errors.Wrap(err, "could not assume role")
		}
	} else {
		sess, err = session.NewSession(config)
		if err != nil {
			return nil, errors.Wrap(err, "could not establish a session")
		}
	}

	provider.session = sess

	// Handle DescribeRegions call with fallback for assume_role_name case
	var regions *ec2.DescribeRegionsOutput
	rc := ec2.New(sess)
	regions, err = rc.DescribeRegions(&ec2.DescribeRegionsInput{})

	if err != nil && options.AssumeRoleName != "" && len(options.AccountIds) > 0 {
		// Base user doesn't have DescribeRegions permission, try with assumed role
		tempSession, err := createAssumedRoleSession(options, sess, config)
		if err != nil {
			return nil, errors.Wrap(err, "could not create assumed role session")
		}

		// Use assumed role session for DescribeRegions
		tempRC := ec2.New(tempSession)
		regions, err = tempRC.DescribeRegions(&ec2.DescribeRegionsInput{})
		if err != nil {
			return nil, errors.Wrap(err, "could not get list of regions even with assumed role")
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "could not get list of regions")
	}

	provider.regions = regions

	provider.initServices(sess)
	return provider, nil
}

func createAssumedRoleSession(options *ProviderOptions, sess *session.Session, config *aws.Config) (*session.Session, error) {
    if len(options.AccountIds) == 0 {
        return nil, errors.New("no account IDs provided for assume role")
    }
    stsClient := sts.New(sess)
    roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", options.AccountIds[0], options.AssumeRoleName)

    roleInput := &sts.AssumeRoleInput{
        RoleArn: aws.String(roleArn),
    }

    if options.AssumeRoleSessionName != "" {
        roleInput.RoleSessionName = aws.String(options.AssumeRoleSessionName)
    } else {
        roleInput.RoleSessionName = aws.String("cloudlist-session")
    }

    if options.ExternalId != "" {
        roleInput.ExternalId = aws.String(options.ExternalId)
    }

    assumeRoleOutput, err := stsClient.AssumeRole(roleInput)
    if err != nil {
        return nil, errors.Wrap(err, "failed to assume role for DescribeRegions")
    }

    assumedCredentials := assumeRoleOutput.Credentials
    tempSession, err := session.NewSession(&aws.Config{
        Credentials: credentials.NewStaticCredentials(
            *assumedCredentials.AccessKeyId,
            *assumedCredentials.SecretAccessKey,
            *assumedCredentials.SessionToken,
        ),
        Region: config.Region,
    })
    if err != nil {
        return nil, errors.Wrap(err, "could not create assumed role session for DescribeRegions")
    }
    return tempSession, nil
}

func (p *Provider) initServices(sess *session.Session) {
	services := p.options.Services

	if services.Has("ec2") || services.Has("instance") {
		p.ec2Client = ec2.New(sess)
	}
	if services.Has("route53") {
		p.route53Client = route53.New(sess)
	}
	if services.Has("s3") {
		p.s3Client = s3.New(sess)
	}
	if services.Has("ecs") {
		p.ecsClient = ecs.New(sess)
	}
	if services.Has("eks") {
		p.eksClient = eks.New(sess)
	}
	if services.Has("lambda") {
		p.lambdaClient = lambda.New(sess)
	}
	if services.Has("apigateway") {
		p.apiGateway = apigateway.New(sess)
	}
	if services.Has("alb") {
		p.albClient = elbv2.New(sess)
	}
	if services.Has("elb") {
		p.elbClient = elb.New(sess)
	}
	if services.Has("lightsail") {
		p.lightsailClient = lightsail.New(sess)
	}
	if services.Has("cloudfront") {
		p.cloudFrontClient = cloudfront.New(sess)
	}
}

const providerName = "aws"
const apiAccessKey = "aws_access_key"
const apiSecretKey = "aws_secret_key"
const sessionToken = "aws_session_token"
const assumeRoleName = "assume_role_name"
const assumeRoleArn = "assume_role_arn"
const externalId = "external_id"
const assumeRoleSessionName = "assume_role_session_name"
const accountIds = "account_ids"

// Name returns the name of the provider
func (p *Provider) Name() string {
	return providerName
}

// ID returns the name of the provider id
func (p *Provider) ID() string {
	return p.options.Id
}

// Services returns the provider services
func (p *Provider) Services() []string {
	return p.options.Services.Keys()
}

type result struct {
	resources *schema.Resources
	err       error
}

type getResourcesFunc func(context.Context) (*schema.Resources, error)

func worker(ctx context.Context, fn getResourcesFunc, ch chan<- result) {
	resources, err := fn(ctx)
	ch <- result{resources, err}
}

func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	finalResources := schema.NewResources()

	var workersWaitGroup sync.WaitGroup
	results := make(chan result)

	assignWorker := func(fn getResourcesFunc) {
		workersWaitGroup.Add(1)
		go func() {
			defer workersWaitGroup.Done()
			worker(ctx, fn, results)
		}()
	}

	if p.ec2Client != nil {
		ec2provider := &instanceProvider{ec2Client: p.ec2Client, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(ec2provider.GetResource)
	}
	if p.route53Client != nil {
		route53Provider := &route53Provider{route53: p.route53Client, options: *p.options, session: p.session}
		assignWorker(route53Provider.GetResource)
	}
	if p.s3Client != nil {
		s3Provider := &s3Provider{s3: p.s3Client, options: *p.options, session: p.session}
		assignWorker(s3Provider.GetResource)
	}
	if p.ecsClient != nil {
		ecsProvider := &ecsProvider{ecsClient: p.ecsClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(ecsProvider.GetResource)
	}
	if p.eksClient != nil {
		eksProvider := &eksProvider{eksClient: p.eksClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(eksProvider.GetResource)
	}
	if p.apiGateway != nil && p.lambdaClient != nil {
		lamdaAndApiGatewayProvider := &lambdaAndapiGatewayProvider{apiGateway: p.apiGateway, lambdaClient: p.lambdaClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(lamdaAndApiGatewayProvider.GetResource)
	}
	if p.albClient != nil {
		albProvider := &elbV2Provider{albClient: p.albClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(albProvider.GetResource)
	}
	if p.elbClient != nil {
		elbProvider := &elbProvider{elbClient: p.elbClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(elbProvider.GetResource)
	}
	if p.lightsailClient != nil {
		lsRegions, err := p.lightsailClient.GetRegions(&lightsail.GetRegionsInput{})
		if err == nil {
			lightsailProvider := &lightsailProvider{lsClient: p.lightsailClient, options: *p.options, session: p.session, regions: lsRegions.Regions}
			assignWorker(lightsailProvider.GetResource)
		}
	}
	if p.cloudFrontClient != nil {
		cloudfrontProvider := &cloudfrontProvider{cloudFrontClient: p.cloudFrontClient, options: *p.options, session: p.session}
		assignWorker(cloudfrontProvider.GetResource)
	}

	go func() {
		workersWaitGroup.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			continue
		}
		finalResources.Merge(result.resources)
	}
	return finalResources, nil
}

// Verify checks if the provider is valid using simple API calls
func (p *Provider) Verify(ctx context.Context) error {
	err := p.verify()
	if err == nil {
		return nil
	}

	if p.options.AssumeRoleName != "" && len(p.options.AccountIds) > 0 {
		tempSession, err := createAssumedRoleSession(p.options, p.session, p.session.Config)
		if err != nil {
			return err
		}

		p.initServices(tempSession)
		err = p.verify()
		if err != nil {
			return err
		}
		return nil
	}
	return err
}

func (p *Provider) verify() error {
	var success bool

	// Try EC2 DescribeRegions (lightweight operation)
	if p.ec2Client != nil {
		_, err := p.ec2Client.DescribeRegions(&ec2.DescribeRegionsInput{})
		if err == nil {
			success = true
		}
	}

	// Try other services with simple operations if EC2 failed
	if !success && p.route53Client != nil {
		_, err := p.route53Client.ListHostedZones(&route53.ListHostedZonesInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.s3Client != nil {
		_, err := p.s3Client.ListBuckets(&s3.ListBucketsInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.lambdaClient != nil {
		_, err := p.lambdaClient.ListFunctions(&lambda.ListFunctionsInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.apiGateway != nil {
		_, err := p.apiGateway.GetRestApis(&apigateway.GetRestApisInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.albClient != nil {
		_, err := p.albClient.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.elbClient != nil {
		_, err := p.elbClient.DescribeLoadBalancers(&elb.DescribeLoadBalancersInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.lightsailClient != nil {
		_, err := p.lightsailClient.GetRegions(&lightsail.GetRegionsInput{})
		if err == nil {
			success = true
		}
	}

	if !success && p.cloudFrontClient != nil {
		_, err := p.cloudFrontClient.ListDistributions(&cloudfront.ListDistributionsInput{})
		if err == nil {
			success = true
		}
	}

	if success {
		return nil
	}
	return errors.New("failed to verify AWS credentials: no accessible services found")
}
