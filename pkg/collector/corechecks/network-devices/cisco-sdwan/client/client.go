// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Cisco SD-WAN API client
package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const timeFormat = "2006-01-02T15:04:05"

// Client is an HTTP Cisco SDWAN client.
type Client struct {
	httpClient          *http.Client
	endpoint            string
	token               string
	tokenExpiry         time.Time
	username            string
	password            string
	authenticationMutex *sync.Mutex
	maxRetries          int
}

// TODO: implement token rotation / handle invalidation, its not working
// TODO: implement API pagination

// NewClient creates a new Cisco SDWAN HTTP client.
func NewClient(hostname, username, password string, useHTTPS bool, insecure bool) (*Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	httpClient := http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
		Jar:       cookieJar,
	}

	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}

	endpoint := url.URL{
		Scheme: scheme,
		Host:   hostname,
	}

	client := &Client{
		httpClient:          &httpClient,
		endpoint:            endpoint.String(),
		username:            username,
		password:            password,
		authenticationMutex: &sync.Mutex{},
		maxRetries:          3,
	}

	return client, nil
}

// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader) (*http.Request, error) {
	return http.NewRequest(method, client.endpoint+uri, body)
}

// do exec a request
func (client *Client) do(req *http.Request) ([]byte, int, error) {
	// Cross-forgery token
	req.Header.Add("X-XSRF-TOKEN", client.token)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}

	defer resp.Body.Close()

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

	for attempts := 0; attempts < client.maxRetries; attempts++ {
		err = client.authenticate()
		if err != nil {
			return nil, err
		}

		bytes, statusCode, err = client.do(req)

		if statusCode == 401 { // this does not work, cisco returns 200 when auth is invalid.......
			// Auth is invalid, clearing token to trigger authentication on next retry
			client.clearAuth()
			continue
		}

		if err != nil || statusCode >= 400 { // FIXME
			continue
		}
	}

	return bytes, nil
}

func (client *Client) getMoreEntries(endpoint string, params map[string]string, pageInfo PageInfo) ([][]byte, error) {
	var responses [][]byte
	// currentPageInfo := pageInfo

	// TODO : implement refetching on pagination
	// for currentPageInfo.MoreEntries {}

	return responses, nil
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

	if data.PageInfo.MoreEntries {
		// TODO: get all entries
		fmt.Println("More entries available")
	}

	return &data, nil
}

// GetDevices get all devices from this SD-WAN network
func (client *Client) GetDevices() ([]Device, error) {
	devices, err := get[Device](client, "/dataservice/device", nil)
	if err != nil {
		return nil, err
	}
	return devices.Data, nil
}

// GetDevicesCounters get all devices from this SD-WAN network
func (client *Client) GetDevicesCounters() ([]DeviceCounters, error) {
	counters, err := get[DeviceCounters](client, "/dataservice/device/counters", nil)
	if err != nil {
		return nil, err
	}
	return counters.Data, nil
}

// GetVEdgeInterfaces gets all Viptela device interfaces
func (client *Client) GetVEdgeInterfaces() ([]InterfaceState, error) {
	params := map[string]string{
		"count": "1000", // TODO: make this configurable
	}

	interfaces, err := get[InterfaceState](client, "/dataservice/data/device/state/Interface", params)
	if err != nil {
		return nil, err
	}
	return interfaces.Data, nil
}

// GetCEdgeInterfaces gets all Cisco device interfaces
func (client *Client) GetCEdgeInterfaces() ([]CEdgeInterfaceState, error) {
	params := map[string]string{
		"count": "1000", // TODO: make this configurable
	}

	interfaces, err := get[CEdgeInterfaceState](client, "/dataservice/data/device/state/CEdgeInterface", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetInterfacesMetrics gets interface metrics
func (client *Client) GetInterfacesMetrics() ([]InterfaceStats, error) {
	endDate := time.Now().UTC()
	startDate := endDate.Add(-time.Minute * 20)

	params := map[string]string{
		"startDate": startDate.Format(timeFormat),
		"endDate":   endDate.Format(timeFormat),
		"timeZone":  "UTC",
		"count":     "1000", // TODO: make this configurable
	}

	interfaces, err := get[InterfaceStats](client, "/dataservice/data/device/statistics/interfacestatistics", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetDeviceHardwareMetrics gets device hardware metrics
func (client *Client) GetDeviceHardwareMetrics() ([]DeviceStatistics, error) {
	endDate := time.Now().UTC()
	startDate := endDate.Add(-time.Minute * 20)

	params := map[string]string{
		"startDate": startDate.Format(timeFormat),
		"endDate":   endDate.Format(timeFormat),
		"timeZone":  "UTC",
		"count":     "1000", // TODO: make this configurable
	}

	interfaces, err := get[DeviceStatistics](client, "/dataservice/data/device/statistics/devicesystemstatusstatistics", params)
	if err != nil {
		return nil, err
	}

	return interfaces.Data, nil
}

// GetApplicationAwareRoutingMetrics gets application aware routing metrics
func (client *Client) GetApplicationAwareRoutingMetrics() ([]AppRouteStatistics, error) {
	endDate := time.Now().UTC()
	startDate := endDate.Add(-time.Minute * 20)

	params := map[string]string{
		"startDate": startDate.Format(timeFormat),
		"endDate":   endDate.Format(timeFormat),
		"timeZone":  "UTC",
		"count":     "1000", // TODO: make this configurable
	}

	appRoutes, err := get[AppRouteStatistics](client, "/dataservice/data/device/statistics/approutestatsstatistics", params)
	if err != nil {
		return nil, err
	}

	return appRoutes.Data, nil
}

// GetControlConnectionsState gets control connection states
func (client *Client) GetControlConnectionsState() ([]ControlConnections, error) {
	params := map[string]string{
		"count": "1000", // TODO: make this configurable
	}

	controlConnections, err := get[ControlConnections](client, "/dataservice/data/device/state/ControlConnection", params)
	if err != nil {
		return nil, err
	}

	return controlConnections.Data, nil
}

// GetOMPPeersState get OMP peer states
func (client *Client) GetOMPPeersState() ([]OMPPeer, error) {
	params := map[string]string{
		"count": "1000", // TODO: make this configurable
	}

	ompPeers, err := get[OMPPeer](client, "/dataservice/data/device/state/OMPPeer", params)
	if err != nil {
		return nil, err
	}

	return ompPeers.Data, nil
}

// GetBFDSessionsState gets BFD session states
func (client *Client) GetBFDSessionsState() ([]BFDSession, error) {
	params := map[string]string{
		"count": "1000", // TODO: make this configurable
	}

	bfdSessions, err := get[BFDSession](client, "/dataservice/data/device/state/BFDSessions", params)
	if err != nil {
		return nil, err
	}

	return bfdSessions.Data, nil
}

// Login logs in to the Cisco SDWAN API
func (client *Client) login() error {
	authPayload := url.Values{}
	authPayload.Set("j_username", client.username)
	authPayload.Set("j_password", client.password)

	// Request to /j_security_check to obtain session cookie
	req, err := client.newRequest("POST", "/j_security_check", strings.NewReader(authPayload.Encode()))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	httpRes, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer httpRes.Body.Close()

	if httpRes.StatusCode != 200 {
		return fmt.Errorf("authentication failed, status code: %v", httpRes.StatusCode)
	}

	bodyBytes, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return err
	}

	if len(bodyBytes) > 0 {
		return fmt.Errorf("invalid credentials")
	}

	// Request to /dataservice/client/token to obtain csrf prevention token
	req, err = client.newRequest("GET", "/dataservice/client/token", nil)
	if err != nil {
		return err
	}
	httpRes, err = client.httpClient.Do(req)
	if err != nil {
		return err
	}

	if httpRes.StatusCode != 200 {
		return fmt.Errorf("failed to retrieve csrf prevention token, status code: %v", httpRes.StatusCode)
	}
	defer httpRes.Body.Close()

	token, _ := io.ReadAll(httpRes.Body)
	if string(token) == "" {
		return fmt.Errorf("no csrf prevention token in payload")
	}

	client.token = string(token)
	client.tokenExpiry = time.Now().Add(time.Hour * 1)
	return nil
}

// Login if no token available.
func (client *Client) authenticate() error {
	now := time.Now()

	client.authenticationMutex.Lock()
	defer client.authenticationMutex.Unlock()

	if client.token == "" || client.tokenExpiry.Before(now) {
		return client.login()
	}
	return nil
}

func (client *Client) clearAuth() {
	client.token = ""
}
