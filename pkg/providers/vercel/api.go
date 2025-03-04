package vercel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
)

type vercelClient struct {
	config     *newClientConfig
	url        string
	httpClient *http.Client
}

type newClientConfig struct {
	Token  string
	Teamid string
}

func newAPIClient(config newClientConfig) *vercelClient {
	return &vercelClient{
		config: &config,
		url:    "https://api.vercel.com",
		httpClient: &http.Client{
			Transport: &http.Transport{},
			Timeout:   60 * time.Second,
		},
	}
}

type apiRequest struct {
	Method         string
	Path           string
	Body           interface{}
	Query          url.Values
	ResponseTarget interface{}
}

func newApiRequest(method string, path string, ResponseTarget interface{}) apiRequest {
	return apiRequest{
		Method:         method,
		Path:           path,
		Body:           nil,
		Query:          url.Values{},
		ResponseTarget: ResponseTarget,
	}
}

// Call the Vercel API and unmarshal its response directly
func (c *vercelClient) Call(req apiRequest) error {
	path := req.Path
	query := req.Query.Encode()
	if query != "" {
		path = fmt.Sprintf("%s?%s", path, query)
	}

	httpResponse, err := c.request(req.Method, path, req.Body)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()
	if req.ResponseTarget == nil {
		return nil
	}
	err = json.NewDecoder(httpResponse.Body).Decode(req.ResponseTarget)
	if err != nil {
		return errors.Wrap(err, "unable to decode response body")
	}
	return nil
}

// Perform a request and return its response
func (c *vercelClient) request(method string, path string, body interface{}) (*http.Response, error) {
	payload, err := marshalBody(body)
	if err != nil {
		return nil, errors.Wrap(err, "unable to marshal request body")
	}
	req, err := http.NewRequest(method, fmt.Sprintf("%s%s", c.url, path), payload)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create request")
	}
	req.Header.Set("User-Agent", "cloudlist-go")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.config.Token))

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "unable to perform request")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var responseBody map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&responseBody)
		if err != nil {
			return nil, fmt.Errorf("response returned status code %d, path: %s", res.StatusCode, path)
		}

		// Try to prettyprint the response body
		// If that is not possible we return the raw body
		pretty, err := json.MarshalIndent(responseBody, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("response returned status code %d: %+v, path: %s", res.StatusCode, responseBody, path)
		}
		return nil, fmt.Errorf("response returned status code %d: %+v, path: %s", res.StatusCode, string(pretty), path)
	}
	return res, nil

}

// JSON marshal the body if present
func marshalBody(body interface{}) (io.Reader, error) {
	var payload io.Reader = nil
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewBuffer(b)
	}
	return payload, nil
}

// Endpoints implementations below
// ------------------------------

func (c *vercelClient) ListProjects(req ListProjectsRequest) (res ListProjectsResponse, err error) {
	apiRequest := newApiRequest("GET", "/v8/projects", &res)
	if c.config.Teamid != "" {
		apiRequest.Query.Add("teamId", c.config.Teamid)
	}
	err = c.Call(apiRequest)
	if err != nil {
		return ListProjectsResponse{}, fmt.Errorf("unable to fetch projects: %w", err)
	}
	return res, nil
}
