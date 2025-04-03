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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, client.endpoint+uri, body)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// do exec a request with authentication
func (client *Client) do(req *http.Request) ([]byte, int, error) {
	// // Cross-forgery token
	client.authenticationMutex.Lock()
	req.Header.Add("X-CSRF-TOKEN", client.token)
	client.authenticationMutex.Unlock()

	log.Tracef("Executing Versa api request %s %s", req.Method, req.URL.Path)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	log.Tracef("Executed Versa api request %d %s %s", resp.StatusCode, req.Method, req.URL.Path)

	defer resp.Body.Close()

	if !isAuthenticated(resp.Header) {
		log.Tracef("Versa api request responded with invalid auth %s %s", req.Method, req.URL.Path)
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

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
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

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// get wraps client.get with generic type content and unmarshalling (methods can't use generics)
func get[T Content](client *Client, endpoint string, params map[string]string) (*T, error) {
	bytes, err := client.get(endpoint, params)
	if err != nil {
		return nil, err
	}

	var data T

	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func isValidStatusCode(code int) bool {
	return code >= 200 && code < 400
}
