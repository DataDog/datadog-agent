// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Cisco SD-WAN API client
package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const timeFormat = "2006-01-02T15:04:05"

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

// Client is an HTTP Cisco SDWAN client.
type Client struct {
	httpClient          *http.Client
	endpoint            string
	token               string
	tokenExpiry         time.Time
	username            string
	password            string
	authenticationMutex *sync.Mutex
	maxAttempts         int
	maxPages            int
	maxCount            string // Stored as string to be passed as an HTTP param
	lookback            time.Duration
	backoffEnabled      bool
	maxRetryDuration    time.Duration   // 0 means retries are only bounded by maxAttempts
	ctx                 context.Context // cancels in-flight requests and rate limiter waits
	rateLimiter         *rate.Limiter   // nil means requests are not rate limited
	rateLimitMaxWait    time.Duration
}

// ClientOptions are the functional options for the Cisco SD-WAN client
type ClientOptions func(*Client)

// NewClient creates a new Cisco SD-WAN HTTP client.
func NewClient(endpoint, username, password string, useHTTP bool, options ...ClientOptions) (*Client, error) {
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
		username:            username,
		password:            password,
		authenticationMutex: &sync.Mutex{},
		maxAttempts:         defaultMaxAttempts,
		maxPages:            defaultMaxPages,
		maxCount:            defaultMaxCount,
		lookback:            defaultLookback,
		ctx:                 context.Background(),
	}

	for _, opt := range options {
		opt(client)
	}

	return client, nil
}

func validateParams(endpoint, username, password string) error {
	if endpoint == "" {
		return errors.New("invalid endpoint")
	}
	if username == "" {
		return errors.New("invalid username")
	}
	if password == "" {
		return errors.New("invalid password")
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

// WithBackoff is a functional option to wait with exponential backoff before retrying
// transient failures (network errors, 429 and 5xx), honoring the Retry-After header
func WithBackoff(enabled bool) ClientOptions {
	return func(c *Client) {
		c.backoffEnabled = enabled
	}
}

// WithMaxRetryDuration is a functional option to bound the total time spent retrying a request:
// no new backoff wait is started if it would end after maxRetryDuration
func WithMaxRetryDuration(maxRetryDuration time.Duration) ClientOptions {
	return func(c *Client) {
		c.maxRetryDuration = maxRetryDuration
	}
}

// WithMaxCount is a functional option to set the client max count
func WithMaxCount(maxCount int) ClientOptions {
	return func(c *Client) {
		c.maxCount = strconv.Itoa(maxCount)
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

// WithContext is a functional option to set the context used to cancel requests and rate limiter waits
func WithContext(ctx context.Context) ClientOptions {
	return func(c *Client) {
		c.ctx = ctx
	}
}

// WithRateLimit is a functional option to limit the number of requests sent to the
// Cisco SD-WAN API using a token bucket refilled at requestsPerSecond, holding up to burst tokens.
// A request fails instead of waiting longer than maxWait for a token.
func WithRateLimit(requestsPerSecond float64, burst int, maxWait time.Duration) ClientOptions {
	return func(c *Client) {
		c.rateLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
		c.rateLimitMaxWait = maxWait
	}
}

// waitForRateLimit blocks until the rate limiter allows a new request to be sent,
// the client context is cancelled, or rateLimitMaxWait is reached
func (client *Client) waitForRateLimit() error {
	if client.rateLimiter == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(client.ctx, client.rateLimitMaxWait)
	defer cancel()
	if err := client.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("cisco sd-wan api rate limiter: %w", err)
	}
	return nil
}

// GetDevices get all devices from this SD-WAN network
func (client *Client) GetDevices() ([]Device, error) {
	devices, err := getAllEntries[Device](client, "/dataservice/device", nil)
	if err != nil {
		return nil, err
	}
	return devices.Data, nil
}

// GetDevicesCounters get all devices from this SD-WAN network
func (client *Client) GetDevicesCounters() ([]DeviceCounters, error) {
	counters, err := getAllEntries[DeviceCounters](client, "/dataservice/device/counters", nil)
	if err != nil {
		return nil, err
	}
	return counters.Data, nil
}

// GetVEdgeInterfaces gets all Viptela device interfaces
func (client *Client) GetVEdgeInterfaces() ([]InterfaceState, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	interfaces, err := getAllEntries[InterfaceState](client, "/dataservice/data/device/state/Interface", params)
	if err != nil {
		return nil, err
	}
	return interfaces.Data, nil
}

// GetCEdgeInterfaces gets all Cisco device interfaces
func (client *Client) GetCEdgeInterfaces() ([]CEdgeInterfaceState, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	interfaces, err := getAllEntries[CEdgeInterfaceState](client, "/dataservice/data/device/state/CEdgeInterface", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetInterfacesMetrics gets interface metrics
func (client *Client) GetInterfacesMetrics() ([]InterfaceStats, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	interfaces, err := getAllEntries[InterfaceStats](client, "/dataservice/data/device/statistics/interfacestatistics", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetDeviceHardwareMetrics gets device hardware metrics
func (client *Client) GetDeviceHardwareMetrics() ([]DeviceStatistics, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	interfaces, err := getAllEntries[DeviceStatistics](client, "/dataservice/data/device/statistics/devicesystemstatusstatistics", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetApplicationAwareRoutingMetrics gets application aware routing metrics
func (client *Client) GetApplicationAwareRoutingMetrics() ([]AppRouteStatistics, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	appRoutes, err := getAllEntries[AppRouteStatistics](client, "/dataservice/data/device/statistics/approutestatsstatistics", params)
	if err != nil {
		return nil, err
	}

	return appRoutes.Data, nil
}

// GetControlConnectionsState gets control connection states
func (client *Client) GetControlConnectionsState() ([]ControlConnections, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	controlConnections, err := getAllEntries[ControlConnections](client, "/dataservice/data/device/state/ControlConnection", params)
	if err != nil {
		return nil, err
	}

	return controlConnections.Data, nil
}

// GetOMPPeersState get OMP peer states
func (client *Client) GetOMPPeersState() ([]OMPPeer, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	ompPeers, err := getAllEntries[OMPPeer](client, "/dataservice/data/device/state/OMPPeer", params)
	if err != nil {
		return nil, err
	}

	return ompPeers.Data, nil
}

// GetBFDSessionsState gets BFD session states
func (client *Client) GetBFDSessionsState() ([]BFDSession, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	bfdSessions, err := getAllEntries[BFDSession](client, "/dataservice/data/device/state/BFDSessions", params)
	if err != nil {
		return nil, err
	}

	return bfdSessions.Data, nil
}

// GetHardwareStates gets hardware states
func (client *Client) GetHardwareStates() ([]HardwareEnvironment, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	hardwareStates, err := getAllEntries[HardwareEnvironment](client, "/dataservice/data/device/state/HardwareEnvironment", params)
	if err != nil {
		return nil, err
	}

	return hardwareStates.Data, nil
}

// GetCloudExpressMetrics gets cloud applications metrics
func (client *Client) GetCloudExpressMetrics() ([]CloudXStatistics, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	cloudApplications, err := getAllEntries[CloudXStatistics](client, "/dataservice/data/device/statistics/cloudxstatistics", params)
	if err != nil {
		return nil, err
	}

	return cloudApplications.Data, nil
}

// GetBGPNeighbors gets BGP neighbors
func (client *Client) GetBGPNeighbors() ([]BGPNeighbor, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	bgpNeighbors, err := getAllEntries[BGPNeighbor](client, "/dataservice/data/device/state/BGPNeighbor", params)
	if err != nil {
		return nil, err
	}

	return bgpNeighbors.Data, nil
}

func (client *Client) statisticsTimeRange() (string, string) {
	endDate := timeNow().UTC()
	startDate := endDate.Add(-client.lookback)
	return startDate.Format(timeFormat), endDate.Format(timeFormat)
}
