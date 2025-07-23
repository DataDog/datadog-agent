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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultMaxAttempts = 3
	defaultMaxPages    = 100
	defaultMaxCount    = "2000"
	defaultLookback    = "30minutesAgo"
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
	lookback            string
}

// ClientOptions are the functional options for the Versa client
type ClientOptions func(*Client)

// NewClient creates a new Versa HTTP client.
func NewClient(directorEndpoint string, directorPort int, analyticsEndpoint string, username string, password string, useHTTP bool, options ...ClientOptions) (*Client, error) {
	err := validateParams(directorEndpoint, directorPort, analyticsEndpoint, username, password)
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
		directorAPIPort:     directorPort,
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

func validateParams(directorEndpoint string, directorPort int, analyticsEndpoint, username, password string) error {
	if directorEndpoint == "" {
		return fmt.Errorf("invalid director endpoint")
	}
	if directorPort == 0 {
		return fmt.Errorf("invalid director port")
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
func WithLookback(lookback int) ClientOptions {
	return func(c *Client) {
		c.lookback = createLookbackString(lookback)
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

// GetAppliances retrieves a list of appliances using the general appliance endpoint
func (client *Client) GetAppliances() ([]Appliance, error) {
	var allAppliances []Appliance
	params := map[string]string{
		"limit":  client.maxCount,
		"offset": "0",
	}

	// Make the first request to get the first page and total count
	resp, err := get[ApplianceListResponse](client, "/vnms/appliance/appliance", params, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get appliances: %v", err)
	}
	if resp == nil {
		return nil, errors.New("failed to get appliances: returned nil")
	}

	// Add the first page of appliances
	allAppliances = append(allAppliances, resp.Appliances...)

	// Calculate remaining pages needed
	maxCount, _ := strconv.Atoi(client.maxCount)
	if maxCount <= 0 {
		return nil, fmt.Errorf("invalid max count: %d", maxCount)
	}

	totalPages := (resp.TotalCount + maxCount - 1) / maxCount // calculate total pages, rounding up

	// Paginate through the remaining pages
	for i := 1; i < totalPages; i++ {
		params["offset"] = strconv.Itoa(i * maxCount)

		pageResp, err := get[ApplianceListResponse](client, "/vnms/appliance/appliance", params, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get appliances page %d: %v", i+1, err)
		}
		if pageResp == nil {
			return nil, fmt.Errorf("failed to get appliances page %d: returned nil", i+1)
		}

		allAppliances = append(allAppliances, pageResp.Appliances...)
	}

	// Verify we got the expected number of appliances
	if len(allAppliances) != resp.TotalCount {
		return nil, fmt.Errorf("failed to get all appliances: expected %d, got %d", resp.TotalCount, len(allAppliances))
	}

	return allAppliances, nil
}

// GetInterfaces retrieves a list of interfaces for a specific tenant
func (client *Client) GetInterfaces(tenantName string) ([]Interface, error) {
	if tenantName == "" {
		return nil, fmt.Errorf("tenantName cannot be empty")
	}

	params := map[string]string{
		"tenantName": tenantName,
	}

	resp, err := get[InterfaceListResponse](client, "/vnms/dashboard/health/interface", params, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %v", err)
	}
	if resp == nil {
		return nil, errors.New("failed to get interfaces: returned nil")
	}

	return resp.List.Value, nil
}

// GetInterfaceMetrics retrieves interface metrics for a specific appliance and tenant using pagination
func (client *Client) GetInterfaceMetrics(applianceName string, tenantName string) ([]InterfaceMetrics, error) {
	if applianceName == "" {
		return nil, fmt.Errorf("applianceName cannot be empty")
	}
	if tenantName == "" {
		return nil, fmt.Errorf("tenantName cannot be empty")
	}

	var allMetrics []InterfaceMetrics

	// make the initial request to get the first page and query-id
	initialEndpoint := fmt.Sprintf("/vnms/dashboard/appliance/%s/pageable_interfaces", applianceName)
	params := map[string]string{
		"orgName": tenantName,
		"limit":   client.maxCount,
	}

	resp, err := get[InterfaceMetricsResponse](client, initialEndpoint, params, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface metrics (initial request): %v", err)
	}
	if resp == nil {
		return nil, errors.New("failed to get interface metrics: initial response was nil")
	}

	queryID := resp.QueryID
	if queryID == "" {
		return nil, errors.New("failed to get interface metrics: no query-id returned")
	}
	// always attempt to clean up the query
	defer client.closeQuery(queryID)

	// add the first page of data
	allMetrics = append(allMetrics, resp.Collection.Interfaces...)

	// if the first page has less interfaces than the limit,
	// there's no need to paginate
	maxCount, _ := strconv.Atoi(client.maxCount)
	if len(resp.Collection.Interfaces) < maxCount {
		return allMetrics, nil
	}

	// paginate through remaining data using the query-id
	nextPageEndpoint := "/vnms/dashboard/appliance/next_page_data"
	for range client.maxPages {
		pageParams := map[string]string{
			"queryId": queryID,
		}

		pageResp, err := get[InterfaceMetricsResponse](client, nextPageEndpoint, pageParams, false)
		if err != nil {
			return nil, fmt.Errorf("failed to get interface metrics (pagination): %v", err)
		}
		if pageResp == nil {
			return nil, errors.New("failed to get interface metrics: page response was nil")
		}

		// Add the page data to our results
		allMetrics = append(allMetrics, pageResp.Collection.Interfaces...)

		// If we get a response with less than the limit,
		// we've hit the last page
		if len(pageResp.Collection.Interfaces) < maxCount {
			break
		}
	}

	return allMetrics, nil
}

// closeQuery closes a pageable query to clean up resources
func (client *Client) closeQuery(queryID string) {
	closeEndpoint := "/vnms/dashboard/appliance/close_query"
	params := map[string]string{
		"queryId": queryID,
	}

	// We don't expect any meaningful response from the close endpoint
	_, err := client.get(closeEndpoint, params, false)
	if err != nil {
		log.Debugf("failed to close query %s: %v", queryID, err)
	}
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
func parseSLAMetrics(data [][]interface{}) ([]SLAMetrics, error) {
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
func (client *Client) GetSLAMetrics(tenant string) ([]SLAMetrics, error) {
	analyticsURL := client.buildAnalyticsPath(tenant, "SDWAN", "slam(localsite,remotesite,localaccckt,remoteaccckt,fc)", "tableData", []string{
		"delay",
		"fwdDelayVar",
		"revDelayVar",
		"fwdLossRatio",
		"revLossRatio",
		"pduLossRatio",
	})

	resp, err := get[AnalyticsMetricsResponse](client, analyticsURL, nil, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get SLA metrics: %v", err)
	}
	aaData := resp.AaData
	metrics, err := parseSLAMetrics(aaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SLA metrics: %v", err)
	}
	return metrics, nil
}

// parseLinkStatusMetrics parses the raw AaData response into LinkStatusMetrics structs
func parseLinkStatusMetrics(data [][]interface{}) ([]LinkStatusMetrics, error) {
	var rows []LinkStatusMetrics
	for _, row := range data {
		m := LinkStatusMetrics{}
		if len(row) < 4 {
			return nil, fmt.Errorf("missing columns in row: got %d columns, expected 4", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Site, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Site")
		}
		if m.AccessCircuit, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for AccessCircuit")
		}
		if m.Availability, ok = row[3].(float64); !ok {
			return nil, fmt.Errorf("expected float64 for Availability")
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// GetLinkStatusMetrics retrieves link status metrics from the Versa Analytics API
func (client *Client) GetLinkStatusMetrics(tenant string) ([]LinkStatusMetrics, error) {
	analyticsURL := client.buildAnalyticsPath(tenant, "SDWAN", "linkstatus(site,accckt)", "tableData", []string{
		"availability",
	})

	resp, err := get[AnalyticsMetricsResponse](client, analyticsURL, nil, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get Link Status Metrics: %v", err)
	}
	aaData := resp.AaData
	metrics, err := parseLinkStatusMetrics(aaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Link Status metrics: %v", err)
	}
	return metrics, nil
}

// parseLinkUsageMetrics parses the raw AaData response into LinkUsageMetrics structs
func parseLinkUsageMetrics(data [][]interface{}) ([]LinkUsageMetrics, error) {
	var rows []LinkUsageMetrics
	for _, row := range data {
		m := LinkUsageMetrics{}
		if len(row) < 13 {
			return nil, fmt.Errorf("missing columns in row: got %d columns, expected 13", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Site, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Site")
		}
		if m.AccessCircuit, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for AccessCircuit")
		}
		if m.UplinkBandwidth, ok = row[3].(string); !ok {
			return nil, fmt.Errorf("expected string for UplinkBandwidth")
		}
		if m.DownlinkBandwidth, ok = row[4].(string); !ok {
			return nil, fmt.Errorf("expected string for DownlinkBandwidth")
		}
		if m.Type, ok = row[5].(string); !ok {
			return nil, fmt.Errorf("expected string for Type")
		}
		if m.Media, ok = row[6].(string); !ok {
			return nil, fmt.Errorf("expected string for Media")
		}
		if m.IP, ok = row[7].(string); !ok {
			return nil, fmt.Errorf("expected string for IP")
		}
		if m.ISP, ok = row[8].(string); !ok {
			return nil, fmt.Errorf("expected string for ISP")
		}

		// Floats from index 9–12
		floatFields := []*float64{
			&m.VolumeTx, &m.VolumeRx, &m.BandwidthTx, &m.BandwidthRx,
		}
		for i, ptr := range floatFields {
			if val, ok := row[i+9].(float64); ok {
				*ptr = val
			} else {
				return nil, fmt.Errorf("expected float64 at index %d", i+9)
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// GetLinkUsageMetrics gets link metrics for a Versa tenant
func (client *Client) GetLinkUsageMetrics(tenant string) ([]LinkUsageMetrics, error) {
	analyticsURL := client.buildAnalyticsPath(tenant, "SDWAN", "linkusage(site,accckt,accckt.uplinkBW,accckt.downlinkBW,accckt.type,accckt.media,accckt.ip,accckt.isp)", "tableData", []string{
		"volume-tx",
		"volume-rx",
		"bw-tx",
		"bw-rx",
	})

	resp, err := get[AnalyticsMetricsResponse](client, analyticsURL, nil, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get link usage metrics: %v", err)
	}
	aaData := resp.AaData
	metrics, err := parseLinkUsageMetrics(aaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse link usage metrics: %v", err)
	}
	return metrics, nil
}

// parseTunnelMetrics parses the raw AaData response into TunnelMetrics structs
func parseTunnelMetrics(data [][]interface{}) ([]TunnelMetrics, error) {
	var rows []TunnelMetrics
	for _, row := range data {
		m := TunnelMetrics{}
		// Based on the new structure, we expect 7 columns
		if len(row) != 7 {
			return nil, fmt.Errorf("expected 7 columns, got %d", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Appliance, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Appliance")
		}
		if m.LocalIP, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for LocalIP")
		}
		if m.RemoteIP, ok = row[3].(string); !ok {
			return nil, fmt.Errorf("expected string for RemoteIP")
		}
		if m.VpnProfName, ok = row[4].(string); !ok {
			return nil, fmt.Errorf("expected string for VpnProfName")
		}

		// Handle float metrics from indices 5-6
		if val, ok := row[5].(float64); ok {
			m.VolumeRx = val
		} else {
			return nil, fmt.Errorf("expected float64 for VolumeRx at index 5")
		}
		if val, ok := row[6].(float64); ok {
			m.VolumeTx = val
		} else {
			return nil, fmt.Errorf("expected float64 for VolumeTx at index 6")
		}

		rows = append(rows, m)
	}
	return rows, nil
}

// GetTunnelMetrics retrieves tunnel metrics from the Versa Analytics API
func (client *Client) GetTunnelMetrics(tenant string) ([]TunnelMetrics, error) {
	if tenant == "" {
		return nil, fmt.Errorf("tenant cannot be empty")
	}

	analyticsURL := client.buildAnalyticsPath(tenant, "SYSTEM", "tunnelstats(appliance,ipsecLocalIp,ipsecPeerIp,ipsecVpnProfName)", "tableData", []string{
		"volume-tx",
		"volume-rx",
	})

	resp, err := get[AnalyticsMetricsResponse](client, analyticsURL, nil, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get tunnel metrics: %v", err)
	}
	aaData := resp.AaData
	metrics, err := parseTunnelMetrics(aaData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tunnel metrics: %v", err)
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
func (client *Client) buildAnalyticsPath(tenant string, feature string, query string, queryType string, metrics []string) string {
	baseAnalyticsPath := "/versa/analytics/v1.0.0/data/provider"
	path := fmt.Sprintf("%s/tenants/%s/features/%s", baseAnalyticsPath, tenant, feature)
	params := url.Values{
		"start-date": []string{client.lookback},
		"qt":         []string{queryType},
		"q":          []string{query},
		"ds":         []string{"aggregate"}, // this seems to be the only datastore supported (from docs)
	}
	for _, m := range metrics {
		params.Add("metrics", m)
	}
	return path + "?" + params.Encode()
}

func createLookbackString(lookbackMinutes int) string {
	return fmt.Sprintf("%dminutesAgo", lookbackMinutes)
}
