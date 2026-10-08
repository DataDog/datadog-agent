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
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client/middleware"
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
	maxRetryDuration    time.Duration // 0 means retries are only bounded by maxAttempts
	rateLimiter         *rate.Limiter // nil means requests are not rate limited, applied by the transport
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
		Jar: cookieJar,
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
	}

	for _, opt := range options {
		opt(client)
	}

	// Options may replace the base transport, wrap it once they are all applied. The timeout
	// is enforced below the rate limiter so waiting for a token does not count against it.
	var transport http.RoundTripper = middleware.NewTimeoutTransport(httpClient.Transport, defaultHTTPTimeout*time.Second)
	if client.rateLimiter != nil {
		transport = middleware.NewRateLimitedTransport(transport, client.rateLimiter, client.rateLimitMaxWait)
	}
	httpClient.Transport = transport

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

func WithMaxAttempts(maxAttempts int) ClientOptions {
	return func(c *Client) {
		c.maxAttempts = maxAttempts
	}
}

func WithBackoff(enabled bool) ClientOptions {
	return func(c *Client) {
		c.backoffEnabled = enabled
	}
}

func WithMaxRetryDuration(maxRetryDuration time.Duration) ClientOptions {
	return func(c *Client) {
		c.maxRetryDuration = maxRetryDuration
	}
}

func WithMaxCount(maxCount int) ClientOptions {
	return func(c *Client) {
		c.maxCount = strconv.Itoa(maxCount)
	}
}

func WithMaxPages(maxPages int) ClientOptions {
	return func(c *Client) {
		c.maxPages = maxPages
	}
}

func WithLookback(lookback time.Duration) ClientOptions {
	return func(c *Client) {
		c.lookback = lookback
	}
}

func WithRateLimit(requestsPerSecond float64, burst int, maxWait time.Duration) ClientOptions {
	return func(c *Client) {
		c.rateLimiter = rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
		c.rateLimitMaxWait = maxWait
	}
}

// GetDevices get all devices from this SD-WAN network
func (client *Client) GetDevices(ctx context.Context) ([]Device, error) {
	devices, err := getAllEntries[Device](ctx, client, "/dataservice/device", nil)
	if err != nil {
		return nil, err
	}
	return devices.Data, nil
}

// GetDevicesCounters get all devices from this SD-WAN network
func (client *Client) GetDevicesCounters(ctx context.Context) ([]DeviceCounters, error) {
	counters, err := getAllEntries[DeviceCounters](ctx, client, "/dataservice/device/counters", nil)
	if err != nil {
		return nil, err
	}
	return counters.Data, nil
}

// GetVEdgeInterfaces gets all Viptela device interfaces
func (client *Client) GetVEdgeInterfaces(ctx context.Context) ([]InterfaceState, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	interfaces, err := getAllEntries[InterfaceState](ctx, client, "/dataservice/data/device/state/Interface", params)
	if err != nil {
		return nil, err
	}
	return interfaces.Data, nil
}

// GetCEdgeInterfaces gets all Cisco device interfaces
func (client *Client) GetCEdgeInterfaces(ctx context.Context) ([]CEdgeInterfaceState, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	interfaces, err := getAllEntries[CEdgeInterfaceState](ctx, client, "/dataservice/data/device/state/CEdgeInterface", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetInterfacesMetrics gets interface metrics
func (client *Client) GetInterfacesMetrics(ctx context.Context) ([]InterfaceStats, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	interfaces, err := getAllEntries[InterfaceStats](ctx, client, "/dataservice/data/device/statistics/interfacestatistics", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetDeviceHardwareMetrics gets device hardware metrics
func (client *Client) GetDeviceHardwareMetrics(ctx context.Context) ([]DeviceStatistics, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	interfaces, err := getAllEntries[DeviceStatistics](ctx, client, "/dataservice/data/device/statistics/devicesystemstatusstatistics", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetApplicationAwareRoutingMetrics gets application aware routing metrics
func (client *Client) GetApplicationAwareRoutingMetrics(ctx context.Context) ([]AppRouteStatistics, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	appRoutes, err := getAllEntries[AppRouteStatistics](ctx, client, "/dataservice/data/device/statistics/approutestatsstatistics", params)
	if err != nil {
		return nil, err
	}

	return appRoutes.Data, nil
}

// GetControlConnectionsState gets control connection states
func (client *Client) GetControlConnectionsState(ctx context.Context) ([]ControlConnections, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	controlConnections, err := getAllEntries[ControlConnections](ctx, client, "/dataservice/data/device/state/ControlConnection", params)
	if err != nil {
		return nil, err
	}

	return controlConnections.Data, nil
}

// GetOMPPeersState get OMP peer states
func (client *Client) GetOMPPeersState(ctx context.Context) ([]OMPPeer, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	ompPeers, err := getAllEntries[OMPPeer](ctx, client, "/dataservice/data/device/state/OMPPeer", params)
	if err != nil {
		return nil, err
	}

	return ompPeers.Data, nil
}

// GetBFDSessionsState gets BFD session states
func (client *Client) GetBFDSessionsState(ctx context.Context) ([]BFDSession, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	bfdSessions, err := getAllEntries[BFDSession](ctx, client, "/dataservice/data/device/state/BFDSessions", params)
	if err != nil {
		return nil, err
	}

	return bfdSessions.Data, nil
}

// GetHardwareStates gets hardware states
func (client *Client) GetHardwareStates(ctx context.Context) ([]HardwareEnvironment, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	hardwareStates, err := getAllEntries[HardwareEnvironment](ctx, client, "/dataservice/data/device/state/HardwareEnvironment", params)
	if err != nil {
		return nil, err
	}

	return hardwareStates.Data, nil
}

// GetCloudExpressMetrics gets cloud applications metrics
func (client *Client) GetCloudExpressMetrics(ctx context.Context) ([]CloudXStatistics, error) {
	startDate, endDate := client.statisticsTimeRange()

	params := map[string]string{
		"startDate": startDate,
		"endDate":   endDate,
		"timeZone":  "UTC",
		"count":     client.maxCount,
	}

	cloudApplications, err := getAllEntries[CloudXStatistics](ctx, client, "/dataservice/data/device/statistics/cloudxstatistics", params)
	if err != nil {
		return nil, err
	}

	return cloudApplications.Data, nil
}

// GetBGPNeighbors gets BGP neighbors
func (client *Client) GetBGPNeighbors(ctx context.Context) ([]BGPNeighbor, error) {
	params := map[string]string{
		"count": client.maxCount,
	}

	bgpNeighbors, err := getAllEntries[BGPNeighbor](ctx, client, "/dataservice/data/device/state/BGPNeighbor", params)
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
