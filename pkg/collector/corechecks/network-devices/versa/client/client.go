// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"
)

const (
	defaultMaxAttempts = 3
	defaultMaxPages    = 100
	defaultMaxCount    = "2000"
	defaultLookback    = 30 * time.Minute
	defaultHTTPTimeout = 10
	defaultHTTPScheme  = "https"
)

// Useful for mocking
var timeNow = time.Now

// Client is an HTTP Versa client.
type Client struct {
	httpClient *http.Client
	endpoint   string
	// TODO: remove when OAuth is implemented
	analyticsEndpoint   string
	token               string
	tokenExpiry         time.Time
	username            string
	password            string
	authenticationMutex *sync.Mutex
	maxAttempts         int
	maxPages            int
	maxCount            string // Stored as string to be passed as an HTTP param
	lookback            time.Duration
}

// ClientOptions are the functional options for the Versa client
type ClientOptions func(*Client)

// NewClient creates a new Versa HTTP client.
func NewClient(endpoint, analyticsEndpoint, username, password string, useHTTP bool, options ...ClientOptions) (*Client, error) {
	err := validateParams(endpoint, username, password)
	if err != nil {
		return nil, err
	}

	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: defaultHTTPTimeout * time.Second,
		Jar:     cookieJar,
	}

	scheme := defaultHTTPScheme
	if useHTTP {
		scheme = "http"
	}

	endpointURL := url.URL{
		Scheme: scheme,
		Host:   endpoint,
	}

	client := &Client{
		httpClient:          httpClient,
		endpoint:            endpointURL.String(),
		analyticsEndpoint:   analyticsEndpoint,
		username:            username,
		password:            password,
		authenticationMutex: &sync.Mutex{},
		maxAttempts:         defaultMaxAttempts,
		maxPages:            defaultMaxPages,
		maxCount:            defaultMaxCount,
		lookback:            defaultLookback,
	}

	for _, opt := range options {
		opt(client)
	}

	return client, nil
}

func validateParams(endpoint, username, password string) error {
	if endpoint == "" {
		return fmt.Errorf("invalid endpoint")
	}
	if username == "" {
		return fmt.Errorf("invalid username")
	}
	if password == "" {
		return fmt.Errorf("invalid password")
	}
	return nil
}

// WithTLSConfig is a functional option to set the HTTP Client TLS Config
func WithTLSConfig(insecure bool, CAFile string) (ClientOptions, error) {
	var caCert []byte
	var err error

	if CAFile != "" {
		caCert, err = os.ReadFile(CAFile)
		if err != nil {
			return nil, err
		}
	}

	return func(c *Client) {
		tlsConfig := &tls.Config{}

		if insecure {
			tlsConfig.InsecureSkipVerify = insecure
		}

		if caCert != nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
		}

		c.httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}, nil
}

// WithMaxAttempts is a functional option to set the client max attempts
func WithMaxAttempts(maxAttempts int) ClientOptions {
	return func(c *Client) {
		c.maxAttempts = maxAttempts
	}
}

// WithMaxCount is a functional option to set the client max count
func WithMaxCount(maxCount int) ClientOptions {
	return func(c *Client) {
		c.maxCount = fmt.Sprintf("%d", maxCount)
	}
}

// WithMaxPages is a functional option to set the client max pages
func WithMaxPages(maxPages int) ClientOptions {
	return func(c *Client) {
		c.maxPages = maxPages
	}
}

// WithLookback is a functional option to set the client lookback interval
func WithLookback(lookback time.Duration) ClientOptions {
	return func(c *Client) {
		c.lookback = lookback
	}
}

// GetAppliancesLite retrieves a list of appliances in a paginated manner
func (client *Client) GetAppliancesLite() ([]ApplianceLite, error) {
	var appliances []ApplianceLite

	params := map[string]string{
		"limit":  client.maxCount,
		"offset": "0",
	}

	resp, err := get[ApplianceLiteResponse](client, "/versa/ncs-services/vnms/appliance/appliance/lite", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get appliance lite response: %v", err)
	}
	appliances = resp.Appliances

	for len(appliances) < resp.TotalCount {
		params["offset"] = fmt.Sprintf("%d", len(appliances))
		resp, err = get[ApplianceLiteResponse](client, "/versa/ncs-services/vnms/appliance/appliance/lite", params)
		if err != nil {
			return nil, fmt.Errorf("failed to get appliance lite response: %v", err)
		}
		appliances = append(appliances, resp.Appliances...)
	}

	return appliances, nil
}

// GetControllerMetadata retrieves the controller metadata
func (client *Client) GetControllerMetadata() ([]ControllerStatus, error) {
	resp, err := get[ControllerResponse](client, "/versa/ncs-services/vnms/dashboard/status/headEnds", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get controller metadata: %v", err)
	}

	return resp.ControllerStatuses, nil
}

// GetDirectorStatus retrieves the director status
func (client *Client) GetDirectorStatus() (*DirectorStatus, error) {
	resp, err := get[DirectorStatus](client, "/versa/ncs-services/vnms/dashboard/status/director", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get director status: %v", err)
	}

	return resp, nil
}
