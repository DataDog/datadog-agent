// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Cisco SD-WAN API client
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

const timeFormat = "2006-01-02T15:04:05"

const (
	defaultMaxAttempts = 3
	defaultMaxPages    = 10
	defaultMaxCount    = 1000
	defaultLookback    = 20 * time.Minute
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
}

// ClientOptions contains Client specific options for the Cisco SD-WAN client
type ClientOptions struct {
	Endpoint   string
	Username   string
	Password   string
	MaxRetries int           // Max retries to apply on HTTP errors
	MaxPages   int           // Max number of pages to retrieve from paginated endpoints
	MaxCount   int           // Max entries to get per request
	Lookback   time.Duration // Look back duration to apply when polling statistics endpoints (minutes)
}

// HTTPOptions contains HTTP specific options for the Cisco SD-WAN client
type HTTPOptions struct {
	UseHTTP  bool
	Insecure bool
	Timeout  int
	CAFile   string
}

// NewClient creates a new Cisco SD-WAN HTTP client.
func NewClient(clientOptions ClientOptions, httpOptions HTTPOptions) (*Client, error) {
	if clientOptions.MaxRetries == 0 {
		clientOptions.MaxRetries = defaultMaxAttempts
	}

	if clientOptions.MaxPages == 0 {
		clientOptions.MaxPages = defaultMaxPages
	}

	if clientOptions.MaxCount == 0 {
		clientOptions.MaxCount = defaultMaxCount
	}

	if clientOptions.Lookback == 0 {
		clientOptions.Lookback = defaultLookback
	}

	httpClient, scheme, err := buildHTTPClient(httpOptions)
	if err != nil {
		return nil, err
	}

	endpoint := url.URL{
		Scheme: scheme,
		Host:   clientOptions.Endpoint,
	}

	client := &Client{
		httpClient:          httpClient,
		endpoint:            endpoint.String(),
		username:            clientOptions.Username,
		password:            clientOptions.Password,
		authenticationMutex: &sync.Mutex{},
		maxAttempts:         clientOptions.MaxRetries,
		maxPages:            clientOptions.MaxPages,
		maxCount:            fmt.Sprintf("%d", clientOptions.MaxCount),
		lookback:            clientOptions.Lookback,
	}

	return client, nil
}

func buildHTTPClient(options HTTPOptions) (*http.Client, string, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: false}

	if options.Insecure {
		tlsConfig.InsecureSkipVerify = options.Insecure
	}

	if options.CAFile != "" {
		caCert, err := os.ReadFile(options.CAFile)
		if err != nil {
			return nil, "", err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig.RootCAs = caCertPool
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", err
	}

	timeout := defaultHTTPTimeout * time.Second
	if options.Timeout > 0 {
		timeout = time.Duration(options.Timeout) * time.Second
	}

	httpClient := http.Client{
		Timeout:   timeout,
		Transport: transport,
		Jar:       cookieJar,
	}

	scheme := defaultHTTPScheme
	if options.UseHTTP {
		scheme = "http"
	}

	return &httpClient, scheme, nil
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

func (client *Client) statisticsTimeRange() (string, string) {
	endDate := timeNow().UTC()
	startDate := endDate.Add(-client.lookback)
	return startDate.Format(timeFormat), endDate.Format(timeFormat)
}
