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
	"github.com/aws/aws-sdk-go/service/apigatewayv2"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lightsail"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
	"github.com/projectdiscovery/gologger"
	sliceutil "github.com/projectdiscovery/utils/slice"
)

var Services = []string{"ec2", "instance", "route53", "s3", "ecs", "eks", "lambda", "apigateway", "apigatewayv2", "alb", "elb", "lightsail", "cloudfront"}

type ProviderOptions struct {
	Id                    string
	AccessKey             string
	SecretKey             string
	Token                 string
	AssumeRoleArn         string
	AssumeRoleSessionName string
	ExternalId            string
	AssumeRoleName        string
	OrgDiscoveryRoleArn   string
	AccountIds            []string
	ExcludeAccountIds     []string
	Services              schema.ServiceMap
	ExtendedMetadata      bool
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

	if orgRoleArn, ok := block.GetMetadata(orgDiscoveryRoleArn); ok {
		p.OrgDiscoveryRoleArn = orgRoleArn
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

	if extendedMetadata, ok := block.GetMetadata("extended_metadata"); ok {
		p.ExtendedMetadata = extendedMetadata == "true"
	}

	if accountIds, ok := block.GetMetadata(accountIds); ok {
		p.AccountIds = sliceutil.Dedupe(strings.Split(accountIds, ","))
	}
	if eids, ok := block.GetMetadata(excludeAccountIds); ok {
		p.ExcludeAccountIds = sliceutil.Dedupe(strings.Split(eids, ","))
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
	apiGatewayV2     *apigatewayv2.ApiGatewayV2
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

	// assume_role_arn replaces the session entirely, so it cannot be combined
	// with multi-account fields that require the raw credentials session
	if options.AssumeRoleArn != "" {
		if options.AssumeRoleName != "" {
			return nil, errors.New("assume_role_arn and assume_role_name cannot be used together")
		}
		if options.OrgDiscoveryRoleArn != "" {
			return nil, errors.New("assume_role_arn and org_discovery_role_arn cannot be used together")
		}
		if len(options.AccountIds) > 0 {
			return nil, errors.New("assume_role_arn and account_ids cannot be used together")
		}
	}

	provider := &Provider{options: options}
	gologger.Debug().Msgf("[cloudlist-debug] aws.New() starting for %d accounts, assume_role_name=%q", len(options.AccountIds), options.AssumeRoleName)
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

	// Discover accounts from AWS Organizations if configured
	if options.OrgDiscoveryRoleArn != "" {
		if options.AssumeRoleName == "" {
			return nil, errors.New("assume_role_name is required when using org_discovery_role_arn")
		}
		discovered, err := provider.discoverOrgAccounts(context.Background(), sess, config)
		if err != nil {
			return nil, errors.Wrap(err, "failed to discover org accounts")
		}
		gologger.Info().Msgf("Discovered %d accounts from AWS Organizations", len(discovered))
		options.AccountIds = sliceutil.Dedupe(append(options.AccountIds, discovered...))
	}

	// Apply exclude filter to account_ids (works with both manual and discovered accounts)
	if len(options.ExcludeAccountIds) > 0 && len(options.AccountIds) > 0 {
		excludeSet := make(map[string]struct{})
		for _, id := range options.ExcludeAccountIds {
			excludeSet[id] = struct{}{}
		}
		filtered := make([]string, 0, len(options.AccountIds))
		for _, id := range options.AccountIds {
			if _, excluded := excludeSet[id]; !excluded {
				filtered = append(filtered, id)
			}
		}
		options.AccountIds = filtered
	}

	if len(options.AccountIds) > 0 && options.AssumeRoleName != "" {
		gologger.Info().Msgf("Will assume role %s in %d accounts", options.AssumeRoleName, len(options.AccountIds))
	}

	// Handle DescribeRegions call with fallback for assume_role_name case
	var regions *ec2.DescribeRegionsOutput
	rc := ec2.New(sess)
	gologger.Debug().Msgf("[cloudlist-debug] calling DescribeRegions (base session)")
	regions, err = rc.DescribeRegions(&ec2.DescribeRegionsInput{})
	gologger.Debug().Msgf("[cloudlist-debug] DescribeRegions returned, err=%v", err)

	if err != nil && options.AssumeRoleName != "" && len(options.AccountIds) > 0 {
		gologger.Debug().Msgf("[cloudlist-debug] DescribeRegions failed, trying fallback with %d accounts", len(options.AccountIds))
		// Base user doesn't have DescribeRegions permission, try with assumed role
		var regionErr error
		for _, accountId := range options.AccountIds {
			gologger.Debug().Msgf("[cloudlist-debug] fallback: AssumeRole for account %s", accountId)
			tempSession, err := createAssumedRoleSession(options, sess, config, accountId)
			if err != nil {
				gologger.Debug().Msgf("[cloudlist-debug] fallback: AssumeRole failed for %s: %v", accountId, err)
				regionErr = err
				continue
			}
			gologger.Debug().Msgf("[cloudlist-debug] fallback: DescribeRegions for account %s", accountId)
			tempRC := ec2.New(tempSession)
			regions, regionErr = tempRC.DescribeRegions(&ec2.DescribeRegionsInput{})
			gologger.Debug().Msgf("[cloudlist-debug] fallback: DescribeRegions for %s returned, err=%v", accountId, regionErr)
			if regionErr == nil {
				break
			}
		}
		if regionErr != nil {
			return nil, errors.Wrap(regionErr, "could not get list of regions with any account")
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "could not get list of regions")
	}

	gologger.Debug().Msgf("[cloudlist-debug] aws.New() got %d regions", len(regions.Regions))
	provider.regions = regions

	provider.initServices(sess)
	gologger.Debug().Msgf("[cloudlist-debug] aws.New() completed successfully")
	return provider, nil
}

func createAssumedRoleSession(options *ProviderOptions, sess *session.Session, config *aws.Config, accountId string) (*session.Session, error) {
	stsClient := sts.New(sess)
	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, options.AssumeRoleName)

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

// discoverOrgAccounts assumes the org discovery role and lists all active
// accounts in the AWS Organization via organizations:ListAccounts.
func (p *Provider) discoverOrgAccounts(ctx context.Context, sess *session.Session, config *aws.Config) ([]string, error) {
	stsClient := sts.New(sess)

	roleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(p.options.OrgDiscoveryRoleArn),
		RoleSessionName: aws.String("cloudlist-org-discovery"),
	}
	if p.options.ExternalId != "" {
		roleInput.ExternalId = aws.String(p.options.ExternalId)
	}

	assumeOut, err := stsClient.AssumeRoleWithContext(ctx, roleInput)
	if err != nil {
		return nil, errors.Wrap(err, "could not assume org discovery role")
	}

	orgSess, err := session.NewSession(&aws.Config{
		Region: config.Region,
		Credentials: credentials.NewStaticCredentials(
			*assumeOut.Credentials.AccessKeyId,
			*assumeOut.Credentials.SecretAccessKey,
			*assumeOut.Credentials.SessionToken,
		),
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create org discovery session")
	}

	orgClient := organizations.New(orgSess)
	var accountIDs []string

	err = orgClient.ListAccountsPagesWithContext(ctx, &organizations.ListAccountsInput{}, func(page *organizations.ListAccountsOutput, lastPage bool) bool {
		for _, acct := range page.Accounts {
			if acct.Status != nil && *acct.Status == "ACTIVE" && acct.Id != nil {
				accountIDs = append(accountIDs, *acct.Id)
			}
		}
		return true
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not list org accounts")
	}

	return accountIDs, nil
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
		p.apiGatewayV2 = apigatewayv2.New(sess)
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
const excludeAccountIds = "exclude_account_ids"
const orgDiscoveryRoleArn = "org_discovery_role_arn"

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
	defer func() {
		if r := recover(); r != nil {
			ch <- result{resources: nil, err: fmt.Errorf("panic in provider worker: %v", r)}
		}
	}()
	resources, err := fn(ctx)
	ch <- result{resources, err}
}

func (p *Provider) Resources(ctx context.Context) (*schema.Resources, error) {
	gologger.Debug().Msgf("[cloudlist-debug] Resources() starting, services=%v, accounts=%d, regions=%d", p.options.Services.Keys(), len(p.options.AccountIds), len(p.regions.Regions))
	finalResources := schema.NewResources()

	var workersWaitGroup sync.WaitGroup
	results := make(chan result)

	workerCount := 0
	assignWorker := func(fn getResourcesFunc) {
		workerCount++
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
	if (p.apiGateway != nil || p.apiGatewayV2 != nil) && p.lambdaClient != nil {
		lambdaAndApiGatewayProvider := &lambdaAndapiGatewayProvider{apiGateway: p.apiGateway, apiGatewayV2: p.apiGatewayV2, lambdaClient: p.lambdaClient, options: *p.options, session: p.session, regions: p.regions}
		assignWorker(lambdaAndApiGatewayProvider.GetResource)
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

	gologger.Debug().Msgf("[cloudlist-debug] Resources() spawned %d workers", workerCount)

	go func() {
		workersWaitGroup.Wait()
		gologger.Debug().Msgf("[cloudlist-debug] Resources() all workers finished")
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			gologger.Debug().Msgf("[cloudlist-debug] Resources() worker error: %v", result.err)
			continue
		}
		finalResources.Merge(result.resources)
	}
	gologger.Debug().Msgf("[cloudlist-debug] Resources() completed, total items=%d", len(finalResources.Items))
	return finalResources, nil
}

// Verify checks if the provider is valid using simple API calls
func (p *Provider) Verify(ctx context.Context) error {
	// Verify org discovery role can assume and list accounts
	if p.options.OrgDiscoveryRoleArn != "" {
		if _, err := p.discoverOrgAccounts(ctx, p.session, p.session.Config); err != nil {
			return errors.Wrap(err, "org discovery verification failed")
		}
	}

	err := p.verify()
	if err == nil {
		return nil
	}

	if p.options.AssumeRoleName != "" && len(p.options.AccountIds) > 0 {
		var mu sync.Mutex
		var failedAccounts []string
		var wg sync.WaitGroup
		sem := make(chan struct{}, 200)

		for _, accountId := range p.options.AccountIds {
			wg.Add(1)
			sem <- struct{}{}
			go func(id string) {
				defer wg.Done()
				defer func() { <-sem }()
				tempSession, err := createAssumedRoleSession(p.options, p.session, p.session.Config, id)
				if err != nil {
					mu.Lock()
					failedAccounts = append(failedAccounts, id)
					mu.Unlock()
					return
				}
				tempProvider := &Provider{options: p.options, session: tempSession}
				tempProvider.initServices(tempSession)
				if err := tempProvider.verify(); err != nil {
					mu.Lock()
					failedAccounts = append(failedAccounts, id)
					mu.Unlock()
				}
			}(accountId)
		}
		wg.Wait()

		if len(failedAccounts) > 0 {
			msg := fmt.Sprintf("failed to assume role %s in accounts: %s", p.options.AssumeRoleName, strings.Join(failedAccounts, ", "))
			if p.options.OrgDiscoveryRoleArn != "" {
				msg += ". Add these to exclude_account_ids if they should not be part of discovery"
			}
			return errors.New(msg)
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

	if !success && p.apiGatewayV2 != nil {
		_, err := p.apiGatewayV2.GetApis(&apigatewayv2.GetApisInput{})
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

type ARNComponents struct {
	Partition    string   // e.g., "aws"
	Service      string   // e.g., "s3", "ec2", "iam"
	Region       string   // e.g., "us-east-1"
	AccountID    string   // e.g., "123456789012"
	Resource     string   // e.g., "bucket/my-bucket" or "instance/i-1234567890abcdef0"
	ResourcePath []string // Resource split by "/" for hierarchical resources
}

// parseARN parses an AWS ARN and returns its components
func parseARN(arn string) *ARNComponents {
	if arn == "" {
		return nil
	}

	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return nil
	}

	components := &ARNComponents{
		Partition: parts[1],
		Service:   parts[2],
		Region:    parts[3],
		AccountID: parts[4],
		Resource:  strings.Join(parts[5:], ":"),
	}

	// Split resource by "/" for hierarchical resources
	components.ResourcePath = strings.Split(components.Resource, "/")

	return components
}

// GetResourceName returns the last component of the resource path
// For example: "cluster/my-cluster" returns "my-cluster"
func (a *ARNComponents) GetResourceName() string {
	if len(a.ResourcePath) > 0 {
		return a.ResourcePath[len(a.ResourcePath)-1]
	}
	return ""
}
