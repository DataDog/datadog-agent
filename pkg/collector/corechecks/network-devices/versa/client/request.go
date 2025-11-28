// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader, useSessionAuth bool) (*http.Request, error) {
	// session auth requires token authentication
	if useSessionAuth {
		return http.NewRequestWithContext(context.Background(), method, client.directorEndpoint+uri, body)
	}
	log.Tracef("Endpoint: %s:%d%s", client.directorEndpoint, client.directorAPIPort, uri)
	return http.NewRequestWithContext(context.Background(), method, fmt.Sprintf("%s:%d%s", client.directorEndpoint, client.directorAPIPort, uri), body)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// do exec a request with authentication
func (client *Client) do(req *http.Request) ([]byte, int, error) {
	log.Tracef("Executing Versa api request %s %s", req.Method, req.URL.Path)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	log.Tracef("Executed Versa api request %d %s %s", resp.StatusCode, req.Method, req.URL.Path)

	defer resp.Body.Close()

	if !isAuthenticated(resp.StatusCode, resp.Header) {
		log.Tracef("Versa api request responded with invalid auth %s %s", req.Method, req.URL.Path)
		// Return 401 on auth errors let the caller decide how to handle it
		return nil, 401, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return body, resp.StatusCode, nil
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use similar
// getWithToken executes GET requests to Director API endpoints with token-based authentication (OAuth or Basic)
func (client *Client) getWithToken(endpoint string, params map[string]string) ([]byte, error) {
	var bytes []byte
	var statusCode int
	var lastErr error

	for attempts := 0; attempts < client.maxAttempts; attempts++ {
		// Authenticate Director API (OAuth needs pre-auth, Basic doesn't)
		err := client.authenticateDirector()
		if err != nil {
			return nil, err
		}

		// Create the request for Director API
		req, err := client.newRequest("GET", endpoint, nil, false)
		if err != nil {
			return nil, err
		}

		// Set Director authentication headers
		switch client.authMethod {
		case authMethodOAuth:
			// use OAuth Bearer token for Director API endpoints
			req.Header.Add("Authorization", "Bearer "+client.directorToken)
		case authMethodBasic:
			// use HTTP Basic authentication for Director API endpoints
			req.SetBasicAuth(client.username, client.password)
		default:
			return nil, fmt.Errorf("unsupported authentication method: %s", client.authMethod)
		}

		query := req.URL.Query()
		for key, value := range params {
			query.Add(key, value)
		}
		req.URL.RawQuery = query.Encode()

		bytes, statusCode, err = client.do(req)

		if err == nil && isValidStatusCode(statusCode) {
			// Got a valid response, stop retrying
			return bytes, nil
		}

		// Handle 401 intelligently based on auth method and attempt number
		if statusCode == 401 {
			if client.authMethod == authMethodOAuth && attempts == 0 {
				// expire token to force refresh on next attempt
				log.Trace("OAuth token rejected, will try refresh on next attempt")
				client.expireDirectorToken()
			} else {
				// OAuth refresh failed, or Basic auth failed, clear and retrys
				log.Trace("Auth failed, clearing tokens for fresh login")
				client.clearDirectorAuth()
			}
		}

		lastErr = err
	}

	log.Tracef("%d error code hitting endpoint %q with error %+v and response: %s", statusCode, endpoint, lastErr, string(bytes))
	return nil, fmt.Errorf("%s http responded with %d code and error %v", endpoint, statusCode, lastErr)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use similar
// getWithSession executes GET requests to Analytics endpoints with session-based authentication
func (client *Client) getWithSession(endpoint string, params map[string]string) ([]byte, error) {
	var bytes []byte
	var statusCode int
	var lastErr error

	for attempts := 0; attempts < client.maxAttempts; attempts++ {
		// Always authenticate session for Analytics endpoints
		err := client.authenticateSession()
		if err != nil {
			return nil, err
		}

		// Create the request for Analytics endpoints
		req, err := client.newRequest("GET", endpoint, nil, true)
		if err != nil {
			return nil, err
		}

		// Set session authentication headers
		req.Header.Add("X-CSRF-TOKEN", client.sessionToken)

		query := req.URL.Query()
		for key, value := range params {
			query.Add(key, value)
		}
		req.URL.RawQuery = query.Encode()

		bytes, statusCode, err = client.do(req)

		if err == nil && isValidStatusCode(statusCode) {
			// Got a valid response, stop retrying
			return bytes, nil
		}

		// session auth doesn't have refresh, just clear and retry
		if statusCode == 401 {
			log.Trace("Session auth failed, clearing session token")
			client.clearSessionAuth()
		}

		lastErr = err
	}

	log.Tracef("%d error code hitting endpoint %q with error %+v and response: %s", statusCode, endpoint, lastErr, string(bytes))
	return nil, fmt.Errorf("%s http responded with %d code and error %v", endpoint, statusCode, lastErr)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// getWithToken wraps client.getWithToken with generic type content and unmarshalling (methods can't use generics)
func getWithToken[T Content](client *Client, endpoint string, params map[string]string) (*T, error) {
	bytes, err := client.getWithToken(endpoint, params)
	if err != nil {
		return nil, err
	}

	var data T

	err = json.Unmarshal(bytes, &data)
	if err != nil {
		log.Tracef("Failed to unmarshal response: %s", err)
		// Log the response body for debugging
		log.Tracef("Response body: %s", string(bytes))
		return nil, err
	}

	return &data, nil
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// getWithSession wraps client.getWithSession with generic type content and unmarshalling (methods can't use generics)
func getWithSession[T Content](client *Client, endpoint string, params map[string]string) (*T, error) {
	bytes, err := client.getWithSession(endpoint, params)
	if err != nil {
		return nil, err
	}

	var data T

	err = json.Unmarshal(bytes, &data)
	if err != nil {
		log.Tracef("Failed to unmarshal response: %s", err)
		// Log the response body for debugging
		log.Tracef("Response body: %s", string(bytes))
		return nil, err
	}

	return &data, nil
}

func isValidStatusCode(code int) bool {
	return code >= 200 && code < 400
}

// getPaginatedAnalytics handles the common pagination pattern for all analytics endpoints
// TODO: perhaps this can be a struct? that way there's no passing around of arguments through
// layers of stuff
func getPaginatedAnalytics[T any](
	client *Client,
	tenant string,
	feature string,
	lookback string,
	query string,
	filterQuery string,
	joinQuery string,
	metrics []string,
	parser func([][]interface{}) ([]T, error),
) ([]T, error) {
	// TODO: store client.maxCount as both string and int?
	maxCount, err := strconv.Atoi(client.maxCount)
	if err != nil {
		return nil, fmt.Errorf("failed to parse maxCount: %v", err)
	}

	var allMetrics []T

	// Paginate through the results
	for page := 0; page < client.maxPages; page++ {
		fromCount := page * maxCount
		analyticsURL := buildAnalyticsPath(tenant, feature, lookback, query, "tableData", filterQuery, joinQuery, metrics, maxCount, fromCount)

		resp, err := getWithSession[AnalyticsMetricsResponse](client, analyticsURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get analytics metrics page %d: %v", page+1, err)
		}

		metrics, err := parser(resp.AaData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse analytics metrics page %d: %v", page+1, err)
		}

		allMetrics = append(allMetrics, metrics...)

		// If we got fewer results than maxCount, we've reached the end
		if len(metrics) < maxCount {
			break
		}
	}

	return allMetrics, nil
}
