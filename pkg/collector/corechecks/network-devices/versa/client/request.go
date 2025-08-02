// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader, useSessionAuth bool) (*http.Request, error) {
	// TODO: remove after triage
	trace := &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			log.Tracef("Getting Conn for: %s", hostPort)
		},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			log.Tracef("Got Conn: %+v", connInfo.Conn.RemoteAddr())
		},
		DNSStart: func(dnsInfo httptrace.DNSStartInfo) {
			log.Tracef("DNS Start: %s", dnsInfo.Host)
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			log.Tracef("DNS Done: Found %+v, Error: %+v", dnsInfo.Addrs, dnsInfo.Err)
		},
		ConnectStart: func(network, addr string) {
			log.Tracef("Connect Start: network %s, addr %s", network, addr)
		},
		ConnectDone: func(network, addr string, err error) {
			log.Tracef("Connect Done: network %s, addr %s, error %+v", network, addr, err)
		},
		TLSHandshakeStart: func() {
			log.Trace("TLS Handshake Started")
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			log.Tracef("TLS Handshake Done State: %+v, Error: %+v", state, err)
		},
	}

	// session auth requires token authentication
	if useSessionAuth {
		return http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), trace), method, client.directorEndpoint+uri, body)
	}
	return http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), trace), method, fmt.Sprintf("%s:%d%s", client.directorEndpoint, client.directorAPIPort, uri), body)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// do exec a request with authentication
func (client *Client) do(req *http.Request) ([]byte, int, error) {
	log.Tracef("Executing Versa api request %s %s", req.Method, req.URL.Path)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		if resp != nil {
			log.Tracef("Error executing Versa api request %d %s %s: Error: %v", resp.StatusCode, req.Method, req.URL.Path, err)
		} else {
			log.Tracef("Error executing Versa api request - %s %s: Error: %v", req.Method, req.URL.Path, err)
		}
		return nil, 0, err
	}
	log.Tracef("Executed Versa api request %d %s %s", resp.StatusCode, req.Method, req.URL.Path)

	defer resp.Body.Close()

	// TODO: should we bring this back with OAuth?
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
func (client *Client) get(endpoint string, params map[string]string, useSessionAuth bool) ([]byte, error) {
	req, err := client.newRequest("GET", endpoint, nil, useSessionAuth)
	if err != nil {
		return nil, err
	}

	// use basic auth
	// TODO: replace with OAuth token
	if useSessionAuth {
		// use token auth
		req.Header.Add("X-CSRF-TOKEN", client.token)
	} else {
		req.SetBasicAuth(client.username, client.password)
	}

	query := req.URL.Query()
	for key, value := range params {
		query.Add(key, value)
	}
	req.URL.RawQuery = query.Encode()

	var bytes []byte
	var statusCode int
	var lastErr error

	for attempts := 0; attempts < client.maxAttempts; attempts++ {
		// TODO: uncomment when OAuth is implemented
		// currently BASIC Auth is being used
		if useSessionAuth {
			err = client.authenticate()
			if err != nil {
				return nil, err
			}
		}

		bytes, statusCode, err = client.do(req)

		if err == nil && isValidStatusCode(statusCode) {
			// Got a valid response, stop retrying
			return bytes, nil
		}
		lastErr = err
	}

	log.Tracef("%d error code hitting endpoint %q response: %q, error: %v", statusCode, endpoint, string(bytes), lastErr)
	return nil, fmt.Errorf("%s http responded with %d code and error %v", endpoint, statusCode, lastErr)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// get wraps client.get with generic type content and unmarshalling (methods can't use generics)
func get[T Content](client *Client, endpoint string, params map[string]string, useSessionAuth bool) (*T, error) {
	bytes, err := client.get(endpoint, params, useSessionAuth)
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
