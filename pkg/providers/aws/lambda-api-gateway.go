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
	"github.com/aws/aws-sdk-go/service/apigatewayv2"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/pkg/errors"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

// lambdaAndapiGatewayProvider is a provider for AWS Lambda and API Gateway resources
type lambdaAndapiGatewayProvider struct {
	options      ProviderOptions
	lambdaClient *lambda.Lambda
	apiGateway   *apigateway.APIGateway
	apiGatewayV2 *apigatewayv2.ApiGatewayV2
	session      *session.Session
	regions      *ec2.DescribeRegionsOutput
}

// GetResource returns all the resources in the store for a provider.
func (ap *lambdaAndapiGatewayProvider) GetResource(ctx context.Context) (*schema.Resources, error) {
	list := schema.NewResources()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, region := range ap.regions.Regions {
		apigatewayClients, apigatewayV2Clients, lambdaClients := ap.getApiGatewayAndLamdaClients(region.RegionName)
		for index := range len(apigatewayClients) {
			wg.Add(1)

			go func(regionName string, gatewayClient *apigateway.APIGateway, gatewayV2Client *apigatewayv2.ApiGatewayV2, lambdaClient *lambda.Lambda) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						mu.Lock()
						errs = append(errs, fmt.Errorf("panic in lambda-apigateway provider: %v", r))
						mu.Unlock()
					}
				}()

				resources := schema.NewResources()

				if gatewayClient != nil {
					if integrationResources, err := ap.listAPIGatewayLambdaIntegrations(regionName, gatewayClient, lambdaClient); err == nil {
						resources.Merge(integrationResources)
					}
					if apiResources, err := ap.listAPIGateways(regionName, gatewayClient); err == nil {
						resources.Merge(apiResources)
					}
				}

				if gatewayV2Client != nil {
					if v2Resources, err := ap.listAPIGatewayV2s(regionName, gatewayV2Client); err == nil {
						resources.Merge(v2Resources)
					}
				}

				mu.Lock()
				list.Merge(resources)
				mu.Unlock()
			}(*region.RegionName, apigatewayClients[index], apigatewayV2Clients[index], lambdaClients[index])
		}
	}
	wg.Wait()
	if len(errs) > 0 && len(list.Items) == 0 {
		return nil, fmt.Errorf("lambda-apigateway: all workers failed: %v", errs)
	}
	return list, nil
}

// listAPIGateways lists all API Gateway resources
func (ap *lambdaAndapiGatewayProvider) listAPIGateways(regionName string, apiGatewayClient *apigateway.APIGateway) (*schema.Resources, error) {
	list := schema.NewResources()

	apis, err := apiGatewayClient.GetRestApis(&apigateway.GetRestApisInput{Limit: aws.Int64(500)})
	if err != nil {
		return nil, errors.Wrap(err, "could not list APIs")
	}

	for _, api := range apis.Items {
		apiBaseURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", *api.Id, regionName)

		// Extract metadata for this API Gateway
		var apiMetadata map[string]string
		if ap.options.ExtendedMetadata {
			apiMetadata = ap.getAPIGatewayMetadata(api, apiGatewayClient, regionName)
		}

		list.Append(&schema.Resource{
			Provider: "aws",
			ID:       *api.Id,
			DNSName:  apiBaseURL,
			Public:   true,
			Service:  "apigateway",
			Metadata: apiMetadata,
		})
	}

	return list, nil
}

// listAPIGatewayLambdaIntegrations lists API Gateway resources that have Lambda integrations
func (ap *lambdaAndapiGatewayProvider) listAPIGatewayLambdaIntegrations(regionName string, apiGatewayClient *apigateway.APIGateway, lambdaClient *lambda.Lambda) (*schema.Resources, error) {
	list := schema.NewResources()

	apis, err := apiGatewayClient.GetRestApis(&apigateway.GetRestApisInput{Limit: aws.Int64(500)})
	if err != nil {
		return nil, errors.Wrap(err, "could not list APIs")
	}

	// List Lambda functions and create a mapping
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

	// Iterate over each API Gateway to find Lambda integrations
	for _, api := range apis.Items {
		apiBaseURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", *api.Id, regionName)

		// Get resources for the API
		resourceReq := &apigateway.GetResourcesInput{
			RestApiId: api.Id,
			Limit:     aws.Int64(100),
		}
		for {
			resources, err := apiGatewayClient.GetResources(resourceReq)
			if err != nil {
				break // Skip this API if we can't get resources
			}

			for _, resource := range resources.Items {
				// List methods for the resource
				for _, method := range resource.ResourceMethods {
					if method == nil || method.HttpMethod == nil {
						continue
					}
					integration, err := apiGatewayClient.GetIntegration(&apigateway.GetIntegrationInput{
						RestApiId:  api.Id,
						ResourceId: resource.Id,
						HttpMethod: aws.String(*method.HttpMethod),
					})
					if err != nil {
						continue
					}
					// Check if the integration type is AWS_PROXY (indicating Lambda integration)
					if integration.Type != nil && *integration.Type == "AWS_PROXY" && integration.Uri != nil {
						functionARN := extractLambdaARN(*integration.Uri)
						if functionName, ok := lambdaFunctionMapping[functionARN]; ok {
							// Create a URL that represents this specific integration
							integrationURL := fmt.Sprintf("%s%s", apiBaseURL, *resource.Path)

							// Build metadata combining API Gateway and Lambda info
							metadata := make(map[string]string)
							if ap.options.ExtendedMetadata {
								metadata["integration_type"] = "api-gateway-lambda"
								metadata["api_gateway_id"] = *api.Id
								metadata["api_gateway_name"] = aws.StringValue(api.Name)
								metadata["lambda_function_name"] = functionName
								metadata["lambda_function_arn"] = functionARN
								metadata["http_method"] = *method.HttpMethod
								metadata["resource_path"] = *resource.Path
								metadata["region"] = regionName

								// Add Lambda function details if available
								if functionDetails, exists := lambdaFunctionDetails[functionARN]; exists {
									if functionDetails.Runtime != nil {
										metadata["lambda_runtime"] = *functionDetails.Runtime
									}
									if functionDetails.Handler != nil {
										metadata["lambda_handler"] = *functionDetails.Handler
									}
								}
							}

							list.Append(&schema.Resource{
								Provider: "aws",
								ID:       fmt.Sprintf("%s-%s", *api.Id, functionARN),
								DNSName:  integrationURL,
								Public:   true,
								Service:  "api-gateway-lambda-integration",
								Metadata: metadata,
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

// listAPIGatewayV2s lists all API Gateway v2 resources (HTTP and WebSocket APIs)
func (ap *lambdaAndapiGatewayProvider) listAPIGatewayV2s(regionName string, apiGatewayV2Client *apigatewayv2.ApiGatewayV2) (*schema.Resources, error) {
	list := schema.NewResources()

	req := &apigatewayv2.GetApisInput{MaxResults: aws.String("100")}
	for {
		apis, err := apiGatewayV2Client.GetApis(req)
		if err != nil {
			return nil, errors.Wrap(err, "could not list API Gateway v2 APIs")
		}

		for _, api := range apis.Items {
			apiBaseURL := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", aws.StringValue(api.ApiId), regionName)

			var apiMetadata map[string]string
			if ap.options.ExtendedMetadata {
				apiMetadata = ap.getAPIGatewayV2Metadata(api, regionName)
			}

			list.Append(&schema.Resource{
				Provider: "aws",
				ID:       aws.StringValue(api.ApiId),
				DNSName:  apiBaseURL,
				Public:   true,
				Service:  "apigatewayv2",
				Metadata: apiMetadata,
			})
		}

		if aws.StringValue(apis.NextToken) == "" {
			break
		}
		req.NextToken = apis.NextToken
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

func (ap *lambdaAndapiGatewayProvider) getApiGatewayAndLamdaClients(region *string) ([]*apigateway.APIGateway, []*apigatewayv2.ApiGatewayV2, []*lambda.Lambda) {
	apiGatewayClients := make([]*apigateway.APIGateway, 0)
	apiGatewayV2Clients := make([]*apigatewayv2.ApiGatewayV2, 0)
	lambdaClients := make([]*lambda.Lambda, 0)

	// Initialize v1 client if available
	if ap.apiGateway != nil {
		albClient := apigateway.New(ap.session, aws.NewConfig().WithRegion(*region))
		apiGatewayClients = append(apiGatewayClients, albClient)
	}

	// Initialize v2 client if available
	if ap.apiGatewayV2 != nil {
		v2Client := apigatewayv2.New(ap.session, aws.NewConfig().WithRegion(*region))
		apiGatewayV2Clients = append(apiGatewayV2Clients, v2Client)
	}

	lambdaClient := lambda.New(ap.session, aws.NewConfig().WithRegion(*region))
	lambdaClients = append(lambdaClients, lambdaClient)

	if ap.options.AssumeRoleName == "" || len(ap.options.AccountIds) < 1 {
		// Ensure all slices have the same length
		for len(apiGatewayClients) < len(lambdaClients) {
			apiGatewayClients = append(apiGatewayClients, nil)
		}
		for len(apiGatewayV2Clients) < len(lambdaClients) {
			apiGatewayV2Clients = append(apiGatewayV2Clients, nil)
		}
		return apiGatewayClients, apiGatewayV2Clients, lambdaClients
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

		if ap.apiGateway != nil {
			apiGatewayClients = append(apiGatewayClients, apigateway.New(assumeSession))
		} else {
			apiGatewayClients = append(apiGatewayClients, nil)
		}

		if ap.apiGatewayV2 != nil {
			apiGatewayV2Clients = append(apiGatewayV2Clients, apigatewayv2.New(assumeSession))
		} else {
			apiGatewayV2Clients = append(apiGatewayV2Clients, nil)
		}

		lambdaClients = append(lambdaClients, lambda.New(assumeSession))
	}
	return apiGatewayClients, apiGatewayV2Clients, lambdaClients
}

func (ap *lambdaAndapiGatewayProvider) getAPIGatewayMetadata(api *apigateway.RestApi, apiGatewayClient *apigateway.APIGateway, regionName string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "api_id", api.Id)
	schema.AddMetadata(metadata, "api_name", api.Name)
	schema.AddMetadata(metadata, "description", api.Description)
	schema.AddMetadata(metadata, "version", api.Version)

	if api.CreatedDate != nil {
		metadata["created_date"] = api.CreatedDate.Format(time.RFC3339)
	}

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
	metadata["region"] = regionName

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

	if len(api.Tags) > 0 {
		if tagString := buildAwsMapTagString(api.Tags); tagString != "" {
			metadata["tags"] = tagString
		}
	}

	return metadata
}

func (ap *lambdaAndapiGatewayProvider) getAPIGatewayV2Metadata(api *apigatewayv2.Api, regionName string) map[string]string {
	metadata := make(map[string]string)

	schema.AddMetadata(metadata, "api_id", api.ApiId)
	schema.AddMetadata(metadata, "api_name", api.Name)
	schema.AddMetadata(metadata, "description", api.Description)
	schema.AddMetadata(metadata, "api_endpoint", api.ApiEndpoint)
	schema.AddMetadata(metadata, "protocol_type", api.ProtocolType)
	schema.AddMetadata(metadata, "route_selection_expression", api.RouteSelectionExpression)
	schema.AddMetadata(metadata, "version", api.Version)

	if api.CreatedDate != nil {
		metadata["created_date"] = api.CreatedDate.Format(time.RFC3339)
	}

	if api.CorsConfiguration != nil {
		if len(api.CorsConfiguration.AllowOrigins) > 0 {
			var origins []string
			for _, origin := range api.CorsConfiguration.AllowOrigins {
				if origin != nil {
					origins = append(origins, aws.StringValue(origin))
				}
			}
			if len(origins) > 0 {
				metadata["cors_origins"] = strings.Join(origins, ",")
			}
		}
	}

	if api.DisableExecuteApiEndpoint != nil {
		metadata["disable_execute_api_endpoint"] = fmt.Sprintf("%t", aws.BoolValue(api.DisableExecuteApiEndpoint))
	}

	metadata["region"] = regionName

	if len(api.Tags) > 0 {
		if tagString := buildAwsMapTagString(api.Tags); tagString != "" {
			metadata["tags"] = tagString
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

func buildAwsMapTagString(tags map[string]*string) string {
	var tagPairs []string
	for key, value := range tags {
		if value != nil {
			tagPairs = append(tagPairs, fmt.Sprintf("%s=%s", key, aws.StringValue(value)))
		}
	}
	return strings.Join(tagPairs, ",")
}
