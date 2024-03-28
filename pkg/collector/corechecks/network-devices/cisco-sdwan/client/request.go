// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, client.endpoint+uri, body)
}

// do exec a request with authentication
func (client *Client) do(req *http.Request) ([]byte, int, error) {
	// Cross-forgery token
	client.authenticationMutex.Lock()
	req.Header.Add("X-XSRF-TOKEN", client.token)
	client.authenticationMutex.Unlock()

	log.Tracef("Executing cisco sd-wan api request %s %s", req.Method, req.URL.Path)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	log.Tracef("Executed cisco sd-wan api request %d %s %s", resp.StatusCode, req.Method, req.URL.Path)

	defer resp.Body.Close()

	if !isAuthenticated(resp.Header) {
		log.Tracef("Cisco sd-wan api request responded with invalid auth %s %s", req.Method, req.URL.Path)
		// clear auth to trigger re-authentication
		client.clearAuth()
		// Return 401 on auth errors
		return nil, 401, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return body, resp.StatusCode, nil
}

// get executes a GET request to the given endpoint with the given query params
func (client *Client) get(endpoint string, params map[string]string) ([]byte, error) {
	req, err := client.newRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	for key, value := range params {
		query.Add(key, value)
	}
	req.URL.RawQuery = query.Encode()

	var bytes []byte
	var statusCode int

	for attempts := 0; attempts < client.maxAttempts; attempts++ {
		err = client.authenticate()
		if err != nil {
			return nil, err
		}

		bytes, statusCode, err = client.do(req)

		if err == nil && isValidStatusCode(statusCode) {
			// Got a valid response, stop retrying
			return bytes, nil
		}
	}

	return nil, fmt.Errorf("%s http responded with %d code", endpoint, statusCode)
}

// get wraps client.get with generic type content and unmarshalling (methods can't use generics)
func get[T Content](client *Client, endpoint string, params map[string]string) (*Response[T], error) {
	bytes, err := client.get(endpoint, params)
	if err != nil {
		return nil, err
	}

	var data Response[T]

	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func isValidStatusCode(code int) bool {
	return code >= 200 && code < 400
}

// getMoreEntries gets all results from paginated endpoints
func getMoreEntries[T Content](client *Client, endpoint string, params map[string]string, pageInfo PageInfo) ([]T, error) {
	var responses []T
	currentPageInfo := pageInfo

	// Loop while API response indicates there is more entries
	for page := 0; currentPageInfo.MoreEntries || currentPageInfo.HasMoreData; page++ {
		// Error if max number of pages is reached
		if page >= client.maxPages {
			return nil, fmt.Errorf("max number of page reached, increase API count or max number of pages")
		}

		log.Tracef("Getting page %d from endpoint %s", page+1+1, endpoint)
		// Update the params to get next API page
		err := updatePaginationParams(currentPageInfo, params)
		if err != nil {
			return nil, err
		}
		log.Tracef("Pagination params for page %d from endpoint %s : %v", page+1+1, endpoint, params)

		// Call the endpoint with the new params
		data, err := get[T](client, endpoint, params)
		if err != nil {
			return nil, err
		}

		responses = append(responses, data.Data...)
		currentPageInfo = data.PageInfo
	}

	return responses, nil
}

// updatePaginationParams updates query params to get next page
func updatePaginationParams(info PageInfo, params map[string]string) error {
	if info.MoreEntries {
		// For endpoints that uses index-based pagination
		endID, err := strconv.Atoi(info.EndID)
		if err != nil {
			return err
		}
		params["startId"] = fmt.Sprintf("%v", endID+1)
	} else if info.HasMoreData {
		// For endpoints that uses scroll-based pagination (ES like)
		params["scrollId"] = info.ScrollID
	}
	return nil
}

// getAllEntries gets all entries from paginated endpoints
func getAllEntries[T Content](client *Client, endpoint string, params map[string]string) (*Response[T], error) {
	data, err := get[T](client, endpoint, params)
	if err != nil {
		return nil, err
	}

	// If API response is paginated, get the rest
	entries, err := getMoreEntries[T](client, endpoint, params, data.PageInfo)
	if err != nil {
		return nil, err
	}

	data.Data = append(data.Data, entries...)

	return data, nil
}
