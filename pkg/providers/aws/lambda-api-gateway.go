package aws

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// apiGatewayProvider is a provider for AWS API Gateway resources
type lambdaAndapiGatewayProvider struct {
	options      ProviderOptions
	lambdaClient *lambda.Lambda
	apiGateway   *apigateway.APIGateway
	session      *session.Session
	regions      *ec2.DescribeRegionsOutput
}

// GetResource returns all the resources in the store for a provider.
func (ap *lambdaAndapiGatewayProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, region := range ap.regions.Regions {
		apigatewayClients, lambdaClients := ap.getApiGatewayAndLamdaClients(region.RegionName)
		for index := range len(apigatewayClients) {
			wg.Add(1)

			go func(regionName string, gatewayClient *apigateway.APIGateway, lambdaClient *lambda.Lambda) {
				defer wg.Done()
				if resources, err := ap.listAPIGatewayResources(regionName, gatewayClient, lambdaClient); err == nil {
					mu.Lock()
					list.Merge(resources)
					mu.Unlock()
				}
			}(*region.RegionName, apigatewayClients[index], lambdaClients[index])
		}
	}
	wg.Wait()
	return list, nil
}

func (ap *lambdaAndapiGatewayProvider) listAPIGatewayResources(regionName string, apiGateway *apigateway.APIGateway, lambdaClient *lambda.Lambda) (*schema.Resources, error) {
	list := schema.NewResources()
	apis, err := apiGateway.GetRestApis(&apigateway.GetRestApisInput{Limit: aws.Int64(500)})
	if err != nil {
		return nil, errors.Wrap(err, "could not list APIs")
	}
	// List Lambda functions and create a mapping of function ARN to function name
	lambdaFunctions, err := ap.getLambdaFunctions(lambdaClient)
	if err != nil {
		return nil, errors.Wrap(err, "could not list Lambda functions")
	}
	lambdaFunctionMapping := make(map[string]string)
	lambdaFunctionDetails := make(map[string]*lambda.FunctionConfiguration)
	for _, lambdaFunction := range lambdaFunctions {
		lambdaFunctionMapping[*lambdaFunction.FunctionArn] = *lambdaFunction.FunctionName
		lambdaFunctionDetails[*lambdaFunction.FunctionArn] = lambdaFunction
	}
	// Iterate over each API Gateway resource
	for _, api := range apis.Items {
		apiBaseURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", *api.Id, regionName)

		// Extract metadata for this API Gateway
		var apiMetadata map[string]string
		if ap.options.ExtendedMetadata {
			apiMetadata = ap.getAPIGatewayMetadata(api, apiGateway, regionName)
		}

		list.Append(&schema.Resource{
			Provider: "aws",
			ID:       *api.Id,
			DNSName:  apiBaseURL,
			Public:   true,
			Service:  "apigateway",
			Metadata: apiMetadata,
		})
		// Get resources for the API
		resourceReq := &apigateway.GetResourcesInput{
			RestApiId: api.Id,
			Limit:     aws.Int64(100),
		}
		for {
			resources, err := apiGateway.GetResources(resourceReq)
			if err != nil {
				return nil, errors.Wrapf(err, "could not get resources for API %s", *api.Id)
			}

			for _, resource := range resources.Items {
				// List methods for the resource
				for _, method := range resource.ResourceMethods {
					if method == nil || method.HttpMethod == nil {
						continue
					}
					integration, err := apiGateway.GetIntegration(&apigateway.GetIntegrationInput{
						RestApiId:  api.Id,
						ResourceId: resource.Id,
						HttpMethod: aws.String(*method.HttpMethod),
					})
					if err != nil {
						continue
					}
					// Check if the integration type is AWS_PROXY (indicating Lambda integration)
					if integration.Type != nil && *integration.Type == "AWS_PROXY" {
						functionARN := extractLambdaARN(*integration.Uri)
						if functionName, ok := lambdaFunctionMapping[functionARN]; ok {
							apiURLWithLambda := fmt.Sprintf("%s/lambda/%s", apiBaseURL, functionName)

							// Extract metadata for this Lambda function
							var lambdaMetadata map[string]string
							if ap.options.ExtendedMetadata {
								if functionDetails, exists := lambdaFunctionDetails[functionARN]; exists {
									lambdaMetadata = ap.getLambdaMetadata(functionDetails, lambdaClient, api)
								}
							}

							list.Append(&schema.Resource{
								Provider: "aws",
								ID:       *api.Id,
								DNSName:  apiURLWithLambda,
								Public:   true,
								Service:  "lambda",
								Metadata: lambdaMetadata,
							})
						}
					}
				}
			}

			if aws.StringValue(resources.Position) == "" {
				break
			}
			resourceReq.SetPosition(*resources.Position)
		}
	}
	return list, nil
}

func (ap *lambdaAndapiGatewayProvider) getLambdaFunctions(lambdaClient *lambda.Lambda) ([]*lambda.FunctionConfiguration, error) {
	var lambdaFunctions []*lambda.FunctionConfiguration
	lambdaReq := &lambda.ListFunctionsInput{MaxItems: aws.Int64(20)}
	for {
		lambdaFuncs, err := lambdaClient.ListFunctions(lambdaReq)
		if err != nil {
			return nil, errors.Wrap(err, "could not list Lambda functions")
		}
		lambdaFunctions = append(lambdaFunctions, lambdaFuncs.Functions...)
		if aws.StringValue(lambdaFuncs.NextMarker) == "" {
			break
		}
		lambdaReq.SetMarker(*lambdaFuncs.NextMarker)
	}
	return lambdaFunctions, nil
}

func (ap *lambdaAndapiGatewayProvider) getApiGatewayAndLamdaClients(region *string) ([]*apigateway.APIGateway, []*lambda.Lambda) {
	apiGatewayClients := make([]*apigateway.APIGateway, 0)
	lambdaClients := make([]*lambda.Lambda, 0)

	albClient := apigateway.New(ap.session, aws.NewConfig().WithRegion(*region))
	apiGatewayClients = append(apiGatewayClients, albClient)

	lambdaClient := lambda.New(ap.session, aws.NewConfig().WithRegion(*region))
	lambdaClients = append(lambdaClients, lambdaClient)

	if ap.options.AssumeRoleName == "" || len(ap.options.AccountIds) < 1 {
		return apiGatewayClients, lambdaClients
	}

	for _, accountId := range ap.options.AccountIds {
		roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, ap.options.AssumeRoleName)
		creds := stscreds.NewCredentials(ap.session, roleARN)

		assumeSession, err := session.NewSession(&aws.Config{
			Region:      region,
			Credentials: creds,
		})
		if err != nil {
			continue
		}
		apiGatewayClients = append(apiGatewayClients, apigateway.New(assumeSession))
		lambdaClients = append(lambdaClients, lambda.New(assumeSession))
	}
	return apiGatewayClients, lambdaClients
}

func (ap *lambdaAndapiGatewayProvider) getAPIGatewayMetadata(api *apigateway.RestApi, apiGatewayClient *apigateway.APIGateway, regionName string) map[string]string {
	metadata := make(map[string]string)

	// Basic API Gateway information
	schema.AddMetadata(metadata, "api_id", api.Id)
	schema.AddMetadata(metadata, "api_name", api.Name)
	schema.AddMetadata(metadata, "description", api.Description)
	schema.AddMetadata(metadata, "version", api.Version)

	if api.CreatedDate != nil {
		metadata["created_date"] = api.CreatedDate.Format(time.RFC3339)
	}

	// API type and endpoint configuration
	if len(api.EndpointConfiguration.Types) > 0 {
		var types []string
		for _, t := range api.EndpointConfiguration.Types {
			if t != nil {
				types = append(types, aws.StringValue(t))
			}
		}
		if len(types) > 0 {
			metadata["endpoint_types"] = strings.Join(types, ",")
		}
	}

	// Extract owner ID from API ID or region/account context
	// API Gateway IDs don't contain account info directly, but we can infer from context
	metadata["region"] = regionName

	// Get stages
	if api.Id != nil {
		if stages, err := apiGatewayClient.GetStages(&apigateway.GetStagesInput{
			RestApiId: api.Id,
		}); err == nil && stages.Item != nil {
			var stageNames []string
			for _, stage := range stages.Item {
				if stage.StageName != nil {
					stageNames = append(stageNames, aws.StringValue(stage.StageName))
				}
			}
			if len(stageNames) > 0 {
				metadata["stages"] = strings.Join(stageNames, ",")
			}
		}
	}

	// Get tags
	if len(api.Tags) > 0 {
		if tagString := buildAPIGatewayTagMap(api.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}

func (ap *lambdaAndapiGatewayProvider) getLambdaMetadata(function *lambda.FunctionConfiguration, lambdaClient *lambda.Lambda, api *apigateway.RestApi) map[string]string {
	metadata := make(map[string]string)

	// Basic Lambda function information
	schema.AddMetadata(metadata, "function_name", function.FunctionName)
	schema.AddMetadata(metadata, "runtime", function.Runtime)
	schema.AddMetadata(metadata, "handler", function.Handler)
	schema.AddMetadata(metadata, "description", function.Description)
	schema.AddMetadata(metadata, "last_modified", function.LastModified)
	schema.AddMetadata(metadata, "version", function.Version)
	schema.AddMetadata(metadata, "execution_role", function.Role)

	if function.FunctionArn != nil {
		arn := aws.StringValue(function.FunctionArn)
		metadata["function_arn"] = arn

		// Extract owner ID from ARN (format: arn:aws:lambda:region:account-id:function:function-name)
		arnParts := strings.Split(arn, ":")
		if len(arnParts) >= 5 && arnParts[4] != "" {
			metadata["owner_id"] = arnParts[4]
		}
	}

	if function.MemorySize != nil {
		metadata["memory_size_mb"] = fmt.Sprintf("%d", aws.Int64Value(function.MemorySize))
	}
	if function.Timeout != nil {
		metadata["timeout_seconds"] = fmt.Sprintf("%d", aws.Int64Value(function.Timeout))
	}
	if function.CodeSize != nil && aws.Int64Value(function.CodeSize) > 0 {
		metadata["code_size_bytes"] = fmt.Sprintf("%d", aws.Int64Value(function.CodeSize))
	}

	// Environment variables count
	if function.Environment != nil && function.Environment.Variables != nil {
		schema.AddMetadataInt(metadata, "env_vars_count", len(function.Environment.Variables))
	}

	// API Gateway association
	if api != nil {
		schema.AddMetadata(metadata, "api_gateway_name", api.Name)
	}

	// Get tags
	if function.FunctionArn != nil {
		if tagOutput, err := lambdaClient.ListTags(&lambda.ListTagsInput{
			Resource: function.FunctionArn,
		}); err == nil && tagOutput.Tags != nil {
			if tagString := buildLambdaTagMap(tagOutput.Tags); tagString != "" {
				metadata["tags"] = tagString
			}
		}
	}

	return metadata
}

// extract Lambda function ARN from integration URI
// Example URI: "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:123456789012:function:my-function/invocations"
func extractLambdaARN(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) >= 5 {
		return parts[3]
	}
	return ""
}

func buildLambdaTagMap(tags map[string]*string) string {
	var tagPairs []string
	for key, value := range tags {
		if value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", key, aws.StringValue(value)))
		}
	}
	return strings.Join(tagPairs, ",")
}

func buildAPIGatewayTagMap(tags map[string]*string) string {
	var tagPairs []string
	for key, value := range tags {
		if value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", key, aws.StringValue(value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
