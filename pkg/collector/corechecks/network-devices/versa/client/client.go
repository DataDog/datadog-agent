// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

import (
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
	httpClient        *http.Client
	directorEndpoint  string
	directorAPIPort   int
	analyticsEndpoint string
	// TODO: replace with OAuth
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
func NewClient(directorEndpoint, analyticsEndpoint, username, password string, useHTTP bool, options ...ClientOptions) (*Client, error) {
	err := validateParams(directorEndpoint, analyticsEndpoint, username, password)
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

	directorEndpointURL := url.URL{
		Scheme: scheme,
		Host:   directorEndpoint,
	}

	analyticsEndpointURL := url.URL{
		Scheme: scheme,
		Host:   analyticsEndpoint,
	}

	client := &Client{
		httpClient:          httpClient,
		directorEndpoint:    directorEndpointURL.String(),
		directorAPIPort:     9182, // TODO: make configurable based on auth type
		analyticsEndpoint:   analyticsEndpointURL.String(),
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

func validateParams(directorEndpoint, analyticsEndpoint, username, password string) error {
	if directorEndpoint == "" {
		return fmt.Errorf("invalid director endpoint")
	}
	if analyticsEndpoint == "" {
		return fmt.Errorf("invalid analytics endpoint")
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

// GetOrganizations retrieves a list of organizations
func (client *Client) GetOrganizations() ([]Organization, error) {
	var organizations []Organization
	resp, err := get[OrganizationListResponse](client, "/vnms/organization/orgs", nil, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations: %v", err)
	}
	organizations = append(organizations, resp.Organizations...)

	// Paginate through the appliances
	maxCount, _ := strconv.Atoi(client.maxCount)
	totalPages := (resp.TotalCount + maxCount - 1) / maxCount // calculate total pages, rounding up if there's any remainder
	for i := 1; i < totalPages; i++ {                         // start from 1 to skip the first page
		params := map[string]string{
			"limit":  client.maxCount,
			"offset": strconv.Itoa(i * maxCount),
		}
		resp, err := get[OrganizationListResponse](client, "/vnms/organization/orgs", params, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get organizations: %v", err)
		}
		if resp == nil {
			return nil, errors.New("failed to get organizations: returned nil")
		}
		organizations = append(organizations, resp.Organizations...)
	}
	if len(organizations) != resp.TotalCount {
		return nil, fmt.Errorf("failed to get organizations: expected %d, got %d", resp.TotalCount, len(organizations))
	}
	return organizations, nil
}

// GetChildAppliancesDetail retrieves a list of appliances with details
func (client *Client) GetChildAppliancesDetail(tenant string) ([]Appliance, error) {
	uri := "/vnms/dashboard/childAppliancesDetail/" + tenant
	var appliances []Appliance
	params := map[string]string{
		"fetch":  "count",
		"limit":  client.maxCount,
		"offset": "0",
	}

	// Get the total count of appliances
	totalCount, err := get[int](client, uri, params, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get appliance detail response: %v", err)
	}
	if totalCount == nil {
		return nil, errors.New("failed to get appliance detail response: returned nil")
	}

	// Paginate through the appliances
	maxCount, _ := strconv.Atoi(client.maxCount)
	totalPages := (*totalCount + maxCount - 1) / maxCount // calculate total pages, rounding up if there's any remainder
	for i := 0; i < totalPages; i++ {
		params["fetch"] = "all"
		params["offset"] = fmt.Sprintf("%d", i*maxCount)
		resp, err := get[[]Appliance](client, uri, params, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get appliance detail response: %v", err)
		}
		if resp == nil {
			return nil, errors.New("failed to get appliance detail response: returned nil")
		}
		appliances = append(appliances, *resp...)
	}

	return appliances, nil
}

// GetDirectorStatus retrieves the director status
func (client *Client) GetDirectorStatus() (*DirectorStatus, error) {
	resp, err := get[DirectorStatus](client, "/vnms/dashboard/vdStatus", nil, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get director status: %v", err)
	}

	return resp, nil
}

// TODO: clean this up to be more generalizable
func parseAaData(data [][]interface{}) ([]SLAMetrics, error) {
	var rows []SLAMetrics
	for _, row := range data {
		m := SLAMetrics{}
		if len(row) != 12 {
			return nil, fmt.Errorf("expected 12 columns, got %d", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for CombinedKey")
		}
		if m.LocalSite, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for LocalSite")
		}
		if m.RemoteSite, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for RemoteSite")
		}
		if m.LocalAccessCircuit, ok = row[3].(string); !ok {
			return nil, fmt.Errorf("expected string for LocalCircuit")
		}
		if m.RemoteAccessCircuit, ok = row[4].(string); !ok {
			return nil, fmt.Errorf("expected string for RemoteCircuit")
		}
		if m.ForwardingClass, ok = row[5].(string); !ok {
			return nil, fmt.Errorf("expected string for ForwardingClass")
		}

		// Floats from index 6–11
		floatFields := []*float64{
			&m.Delay, &m.FwdDelayVar, &m.RevDelayVar,
			&m.FwdLossRatio, &m.RevLossRatio, &m.PDULossRatio,
		}
		for i, ptr := range floatFields {
			if val, ok := row[i+6].(float64); ok {
				*ptr = val
			} else {
				return nil, fmt.Errorf("expected float64 at index %d", i+6)
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// GetSLAMetrics retrieves SLA metrics from the Versa Analytics API
func (client *Client) GetSLAMetrics() ([]SLAMetrics, error) {
	analyticsURL := buildAnalyticsPath("datadog", "SDWAN", "15minutesAgo", "slam(localsite,remotesite,localaccckt,remoteaccckt,fc)", "tableData", []string{
		"delay",
		"fwdDelayVar",
		"revDelayVar",
		"fwdLossRatio",
		"revLossRatio",
		"pduLossRatio",
	})

	resp, err := get[SLAMetricsResponse](client, analyticsURL, nil, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get SLA metrics: %v", err)
	}
	aaData := resp.AaData
	metrics, err := parseAaData(aaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SLA metrics: %v", err)
	}
	return metrics, nil
}

// buildAnalyticsPath constructs a Versa Analytics query path in a cleaner way so multiple metrics can be added.
//
// Parameters:
//   - tenant: tenant name within the environment (e.g., "datadog")
//   - feature: category of analytics metrics (e.g., "SDWAN, "SYSTEM", "CGNAT", etc.).
//   - startDate: relative start date (e.g., "15minutesAgo", "1h", "24h").
//   - query: Versa query expression (e.g., "slam(...columns...)").
//   - queryType: type of query (e.g., "tableData", "table", "summary").
//   - metrics: list of metric strings (e.g., "delay", "fwdLossRatio").
//
// Returns the full encoded URL string.
func buildAnalyticsPath(tenant string, feature string, startDate string, query string, queryType string, metrics []string) string {
	baseAnalyticsPath := "/versa/analytics/v1.0.0/data/provider"
	path := fmt.Sprintf("%s/tenants/%s/features/%s", baseAnalyticsPath, tenant, feature)
	params := url.Values{
		"start-date": []string{startDate},
		"qt":         []string{queryType},
		"q":          []string{query},
		"ds":         []string{"aggregate"}, // this seems to be the only datastore supported (from docs)
	}
	for _, m := range metrics {
		params.Add("metrics", m)
	}
	return path + "?" + params.Encode()
}
