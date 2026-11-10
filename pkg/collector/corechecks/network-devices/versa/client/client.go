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
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultBasicPort   = 9182
	defaultOAuthPort   = 9183
	defaultMaxAttempts = 3
	defaultMaxPages    = 100
	defaultMaxCount    = 2000
	defaultLookback    = "30minutesAgo"
	defaultHTTPTimeout = 10 * time.Second
	defaultHTTPScheme  = "https"

	// QueryTypeTableData returns one aggregate row per entity covering the whole lookback
	// window. This is the default, and historically it was the only behavior.
	QueryTypeTableData = "tableData"
	// QueryTypeTimeseries returns one row per entity per time bucket, so outliers inside
	// the lookback window survive instead of being averaged away.
	QueryTypeTimeseries = "timeseries"

	// defaultAnalyticsGap is the bucket size requested for timeseries queries. Versa only
	// accepts 1MINUTE, 1HOUR and 1DAY here, and it renders at the tenant's configured
	// resolution regardless, so 1MINUTE means "the finest the tenant will give us": 15
	// minutes on a 15 minute tenant, 5 on a 5 minute one.
	defaultAnalyticsGap = "1MINUTE"
	// timeseriesEndDate bounds a timeseries query. tableData queries send no end-date.
	// Versa resolves "today" to now, not to midnight: a 4hoursAgo request against a 15
	// minute tenant comes back with 16 buckets ending at the current quarter hour.
	timeseriesEndDate = "today"
)

// Endpoint identifies a group of Versa API calls whose request settings
// (page size, page count, timeout) can be overridden independently
type Endpoint string

// Endpoints that support request setting overrides. Names mirror the
// collect_* check options where one exists.
const (
	EndpointOrganizations                  Endpoint = "organizations"
	EndpointAppliances                     Endpoint = "appliances"
	EndpointInterfaces                     Endpoint = "interfaces"
	EndpointDirectorStatus                 Endpoint = "director_status"
	EndpointDirectorInterfaceMetrics       Endpoint = "director_interface_metrics"
	EndpointSLAMetrics                     Endpoint = "sla_metrics"
	EndpointLinkStatusMetrics              Endpoint = "link_status_metrics"
	EndpointLinkUsageMetrics               Endpoint = "link_usage_metrics"
	EndpointSiteMetrics                    Endpoint = "site_metrics"
	EndpointApplicationsByApplianceMetrics Endpoint = "applications_by_appliance_metrics"
	EndpointTopUserMetrics                 Endpoint = "top_user_metrics"
	EndpointTunnelMetrics                  Endpoint = "tunnel_metrics"
	EndpointQoSMetrics                     Endpoint = "qos_metrics"
	EndpointCircuitQoSMetrics              Endpoint = "circuit_qos_metrics"
	EndpointDIAMetrics                     Endpoint = "dia_metrics"
	EndpointInterfaceMetrics               Endpoint = "interface_metrics"
)

var allEndpoints = []Endpoint{
	EndpointOrganizations,
	EndpointAppliances,
	EndpointInterfaces,
	EndpointDirectorStatus,
	EndpointDirectorInterfaceMetrics,
	EndpointSLAMetrics,
	EndpointLinkStatusMetrics,
	EndpointLinkUsageMetrics,
	EndpointSiteMetrics,
	EndpointApplicationsByApplianceMetrics,
	EndpointTopUserMetrics,
	EndpointTunnelMetrics,
	EndpointQoSMetrics,
	EndpointCircuitQoSMetrics,
	EndpointDIAMetrics,
	EndpointInterfaceMetrics,
}

// ParseEndpoint validates an endpoint name from configuration
func ParseEndpoint(name string) (Endpoint, error) {
	for _, e := range allEndpoints {
		if string(e) == name {
			return e, nil
		}
	}
	names := make([]string, 0, len(allEndpoints))
	for _, e := range allEndpoints {
		names = append(names, string(e))
	}
	return "", fmt.Errorf("unknown endpoint %q, must be one of: %s", name, strings.Join(names, ", "))
}

// EndpointConfig holds the request settings for an endpoint. When used as an
// override, zero values inherit the client-wide setting.
type EndpointConfig struct {
	MaxCount int
	MaxPages int
	Timeout  time.Duration
}

// Useful for mocking
var timeNow = time.Now

// Client is an HTTP Versa client.
type Client struct {
	httpClient        *http.Client
	directorEndpoint  string
	directorAPIPort   int
	analyticsEndpoint string
	// OAuth token for Director API endpoints
	directorToken       string
	directorTokenExpiry time.Time
	// Session token for Analytics endpoints (always uses session auth)
	sessionToken        string
	sessionTokenExpiry  time.Time
	username            string
	password            string
	clientID            string
	clientSecret        string
	authMethod          authMethod
	authenticationMutex *sync.Mutex
	maxAttempts         int
	maxPages            int
	maxCount            int
	timeout             time.Duration
	endpointOverrides   map[Endpoint]EndpointConfig
	lookback            string
	analyticsQueryType  string
	analyticsGap        string
}

// ClientOptions are the functional options for the Versa client
type ClientOptions func(*Client)

// NewClient creates a new Versa HTTP client.
func NewClient(directorEndpoint string, directorPort int, analyticsEndpoint string, useHTTP bool, authConfig AuthConfig, options ...ClientOptions) (*Client, error) {
	err := validateParams(directorEndpoint, directorPort, analyticsEndpoint)
	if err != nil {
		return nil, err
	}

	// Process authentication configuration (validate and parse)
	authMethod, err := processAuthConfig(authConfig)
	if err != nil {
		return nil, err
	}

	// Set default port based on authentication method if not provided
	if directorPort == 0 {
		if authMethod == authMethodOAuth {
			directorPort = defaultOAuthPort
		} else {
			directorPort = defaultBasicPort
		}
	}

	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: defaultHTTPTimeout,
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
		authMethod:          authMethod,
		username:            authConfig.Username,
		password:            authConfig.Password,
		clientID:            authConfig.ClientID,
		clientSecret:        authConfig.ClientSecret,
		authenticationMutex: &sync.Mutex{},
		maxAttempts:         defaultMaxAttempts,
		maxPages:            defaultMaxPages,
		maxCount:            defaultMaxCount,
		timeout:             defaultHTTPTimeout,
		endpointOverrides:   make(map[Endpoint]EndpointConfig),
		lookback:            defaultLookback,
		analyticsQueryType:  QueryTypeTableData,
		analyticsGap:        defaultAnalyticsGap,
	}

	for _, opt := range options {
		opt(client)
	}
	// Auth requests and any request without an endpoint override use the client-wide timeout
	client.httpClient.Timeout = client.timeout

	return client, nil
}

func validateParams(directorEndpoint string, directorPort int, analyticsEndpoint string) error {
	if directorEndpoint == "" {
		return errors.New("invalid director endpoint")
	}
	if directorPort < 0 {
		return fmt.Errorf("invalid director port: %d", directorPort)
	}
	if analyticsEndpoint == "" {
		return errors.New("invalid analytics endpoint")
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
		c.maxCount = maxCount
	}
}

// WithMaxPages is a functional option to set the client max pages
func WithMaxPages(maxPages int) ClientOptions {
	return func(c *Client) {
		c.maxPages = maxPages
	}
}

// WithTimeout is a functional option to set the client-wide HTTP request timeout
func WithTimeout(timeout time.Duration) ClientOptions {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// WithEndpointConfig is a functional option to override request settings for
// a single endpoint. Zero-valued fields inherit the client-wide setting.
func WithEndpointConfig(endpoint Endpoint, config EndpointConfig) ClientOptions {
	return func(c *Client) {
		c.endpointOverrides[endpoint] = config
	}
}

// endpointConfig resolves the request settings for an endpoint, applying any
// override on top of the client-wide settings
func (client *Client) endpointConfig(endpoint Endpoint) EndpointConfig {
	config := EndpointConfig{
		MaxCount: client.maxCount,
		MaxPages: client.maxPages,
		Timeout:  client.timeout,
	}
	override := client.endpointOverrides[endpoint]
	if override.MaxCount > 0 {
		config.MaxCount = override.MaxCount
	}
	if override.MaxPages > 0 {
		config.MaxPages = override.MaxPages
	}
	if override.Timeout > 0 {
		config.Timeout = override.Timeout
	}
	return config
}

// WithLookback is a functional option to set the client lookback interval
func WithLookback(lookback int) ClientOptions {
	return func(c *Client) {
		c.lookback = createLookbackString(lookback)
	}
}

// WithAnalyticsQueryType is a functional option to set the Analytics query type used by
// every analytics call. Accepts QueryTypeTableData or QueryTypeTimeseries; an unknown
// value leaves the default in place, since the check validates it before reaching here.
func WithAnalyticsQueryType(queryType string) ClientOptions {
	return func(c *Client) {
		if queryType == QueryTypeTableData || queryType == QueryTypeTimeseries {
			c.analyticsQueryType = queryType
		}
	}
}

// WithAnalyticsGap is a functional option to set the bucket size for timeseries queries.
// It has no effect on tableData queries.
func WithAnalyticsGap(gap string) ClientOptions {
	return func(c *Client) {
		if gap != "" {
			// Versa matches the gap exactly, so normalize here rather than sending "1hour"
			// and taking an opaque 4xx on every analytics call.
			c.analyticsGap = strings.ToUpper(gap)
		}
	}
}

// GetOrganizations retrieves a list of organizations
func (client *Client) GetOrganizations() ([]Organization, error) {
	cfg := client.endpointConfig(EndpointOrganizations)
	var organizations []Organization
	resp, err := get[OrganizationListResponse](client, "/vnms/organization/orgs", nil, false, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations: %v", err)
	}
	organizations = append(organizations, resp.Organizations...)

	// Paginate through the appliances
	totalPages := (resp.TotalCount + cfg.MaxCount - 1) / cfg.MaxCount // calculate total pages, rounding up if there's any remainder
	for i := 1; i < totalPages; i++ {                                 // start from 1 to skip the first page
		params := map[string]string{
			"limit":  strconv.Itoa(cfg.MaxCount),
			"offset": strconv.Itoa(i * cfg.MaxCount),
		}
		resp, err := get[OrganizationListResponse](client, "/vnms/organization/orgs", params, false, cfg.Timeout)
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
	cfg := client.endpointConfig(EndpointAppliances)
	uri := "/vnms/dashboard/childAppliancesDetail/" + tenant
	var appliances []Appliance
	params := map[string]string{
		"fetch":  "count",
		"limit":  strconv.Itoa(cfg.MaxCount),
		"offset": "0",
	}

	// Get the total count of appliances
	totalCount, err := get[int](client, uri, params, false, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get appliance detail response: %v", err)
	}
	if totalCount == nil {
		return nil, errors.New("failed to get appliance detail response: returned nil")
	}

	// Paginate through the appliances
	totalPages := (*totalCount + cfg.MaxCount - 1) / cfg.MaxCount // calculate total pages, rounding up if there's any remainder
	for i := 0; i < totalPages; i++ {
		params["fetch"] = "all"
		params["offset"] = strconv.Itoa(i * cfg.MaxCount)
		resp, err := get[[]Appliance](client, uri, params, false, cfg.Timeout)
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
	cfg := client.endpointConfig(EndpointAppliances)
	var allAppliances []Appliance
	params := map[string]string{
		"limit":  strconv.Itoa(cfg.MaxCount),
		"offset": "0",
	}

	// Make the first request to get the first page and total count
	resp, err := get[ApplianceListResponse](client, "/vnms/appliance/appliance", params, false, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get appliances: %v", err)
	}
	if resp == nil {
		return nil, errors.New("failed to get appliances: returned nil")
	}

	// Add the first page of appliances
	allAppliances = append(allAppliances, resp.Appliances...)

	// Calculate remaining pages needed
	maxCount := cfg.MaxCount
	if maxCount <= 0 {
		return nil, fmt.Errorf("invalid max count: %d", maxCount)
	}

	totalPages := (resp.TotalCount + maxCount - 1) / maxCount // calculate total pages, rounding up

	// Paginate through the remaining pages
	for i := 1; i < totalPages; i++ {
		params["offset"] = strconv.Itoa(i * maxCount)

		pageResp, err := get[ApplianceListResponse](client, "/vnms/appliance/appliance", params, false, cfg.Timeout)
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
		return nil, errors.New("tenantName cannot be empty")
	}

	params := map[string]string{
		"tenantName": tenantName,
	}

	cfg := client.endpointConfig(EndpointInterfaces)
	resp, err := get[InterfaceListResponse](client, "/vnms/dashboard/health/interface", params, false, cfg.Timeout)
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
		return nil, errors.New("applianceName cannot be empty")
	}
	if tenantName == "" {
		return nil, errors.New("tenantName cannot be empty")
	}

	cfg := client.endpointConfig(EndpointDirectorInterfaceMetrics)
	var allMetrics []InterfaceMetrics

	// make the initial request to get the first page and query-id
	initialEndpoint := fmt.Sprintf("/vnms/dashboard/appliance/%s/pageable_interfaces", applianceName)
	params := map[string]string{
		"orgName": tenantName,
		"limit":   strconv.Itoa(cfg.MaxCount),
	}

	resp, err := get[InterfaceMetricsResponse](client, initialEndpoint, params, false, cfg.Timeout)
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
	defer client.closeQuery(queryID, cfg.Timeout)

	// add the first page of data
	allMetrics = append(allMetrics, resp.Collection.Interfaces...)

	// if the first page has less interfaces than the limit,
	// there's no need to paginate
	if len(resp.Collection.Interfaces) < cfg.MaxCount {
		return allMetrics, nil
	}

	// paginate through remaining data using the query-id
	nextPageEndpoint := "/vnms/dashboard/appliance/next_page_data"
	for range cfg.MaxPages {
		pageParams := map[string]string{
			"queryId": queryID,
		}

		pageResp, err := get[InterfaceMetricsResponse](client, nextPageEndpoint, pageParams, false, cfg.Timeout)
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
		if len(pageResp.Collection.Interfaces) < cfg.MaxCount {
			break
		}
	}

	return allMetrics, nil
}

// closeQuery closes a pageable query to clean up resources
func (client *Client) closeQuery(queryID string, timeout time.Duration) {
	closeEndpoint := "/vnms/dashboard/appliance/close_query"
	params := map[string]string{
		"queryId": queryID,
	}

	// We don't expect any meaningful response from the close endpoint
	_, err := client.get(closeEndpoint, params, false, timeout)
	if err != nil {
		log.Debugf("failed to close query %s: %v", queryID, err)
	}
}

// GetDirectorStatus retrieves the director status
func (client *Client) GetDirectorStatus() (*DirectorStatus, error) {
	resp, err := get[DirectorStatus](client, "/vnms/dashboard/vdStatus", nil, false, client.endpointConfig(EndpointDirectorStatus).Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get director status: %v", err)
	}

	return resp, nil
}

// GetSLAMetrics retrieves SLA metrics from the Versa Analytics API
func (client *Client) GetSLAMetrics(tenant string) ([]SLAMetrics, error) {
	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointSLAMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"slam(localsite,remotesite,localaccckt,remoteaccckt,fc)",
		"",
		"",
		[]string{
			"delay",
			"fwdDelayVar",
			"revDelayVar",
			"fwdLossRatio",
			"revLossRatio",
			"pduLossRatio",
		},
		slaTimeseriesLayout,
		parseSLAMetrics,
	)
}

// GetPathQoSMetrics retrieves QoS (Class of Service) metrics from the Versa Analytics API
func (client *Client) GetPathQoSMetrics(tenant string) ([]QoSMetrics, error) {
	metrics, err := getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointQoSMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"pathcos(localsitename,remotesitename)",
		"",
		"",
		[]string{
			"betx",        // best effort bytes
			"betxdrop",    // best effort dropped
			"eftx",        // expedited forwarding bytes
			"eftxdrop",    // expedited forwarding dropped
			"aftx",        // assured forwarding bytes
			"aftxdrop",    // assured forwarding dropped
			"nctx",        // network control bytes
			"nctxdrop",    // network control dropped
			"bebandwidth", // best effort bps
			"efbandwidth", // expedited forwarding bw bps
			"afbandwidth", // assured forwarding bw bps
			"ncbandwidth", // network control bw bps
			"volume-tx",   // total volume bytes
			"totaldrop",   // total drops bytes
			"percentdrop", // percent drop bytes
			"bandwidth",   // total bandwidth bps
		},
		pathQoSTimeseriesLayout,
		parsePathQoSMetrics,
	)
	if err != nil {
		return nil, err
	}
	if client.analyticsQueryType == QueryTypeTimeseries {
		computePercentDrop(metrics, func(m *QoSMetrics) (*float64, *float64, *float64, *bool) {
			return &m.VolumeTx, &m.TotalDrop, &m.PercentDrop, &m.PercentDropUnset
		})
	}
	return metrics, nil
}

// GetCircuitQoSMetrics retrieves per access circuit QoS (Class of Service) metrics from
// the Versa Analytics API. Where pathcos reports class of service between a pair of sites,
// cos reports it for each access circuit of a site, which is the branch side view of
// egress queueing.
//
// Versa logs only the main traffic class unless log-fc-stats is enabled under the
// network's class of service configuration, so a tenant without it will see the four
// classes here but no per queue detail.
func (client *Client) GetCircuitQoSMetrics(tenant string) ([]CircuitQoSMetrics, error) {
	if tenant == "" {
		return nil, errors.New("tenant cannot be empty")
	}
	metrics, err := getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointCircuitQoSMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"cos(site,accckt)",
		"",
		"",
		[]string{
			"betx",        // best effort bytes
			"betxdrop",    // best effort dropped
			"eftx",        // expedited forwarding bytes
			"eftxdrop",    // expedited forwarding dropped
			"aftx",        // assured forwarding bytes
			"aftxdrop",    // assured forwarding dropped
			"nctx",        // network control bytes
			"nctxdrop",    // network control dropped
			"bebandwidth", // best effort bps
			"efbandwidth", // expedited forwarding bps
			"afbandwidth", // assured forwarding bps
			"ncbandwidth", // network control bps
			"volume-tx",   // total volume bytes
			"totaldrop",   // total drops bytes
			"percentdrop", // percent drop bytes
			"bandwidth",   // total bandwidth bps
		},
		circuitQoSTimeseriesLayout,
		parseCircuitQoSMetrics,
	)
	if err != nil {
		return nil, err
	}
	if client.analyticsQueryType == QueryTypeTimeseries {
		computePercentDrop(metrics, func(m *CircuitQoSMetrics) (*float64, *float64, *float64, *bool) {
			return &m.VolumeTx, &m.TotalDrop, &m.PercentDrop, &m.PercentDropUnset
		})
	}
	return metrics, nil
}

// computePercentDrop replaces percentdrop in the rows of a timeseries QoS query. Versa
// reports it as zero in every timeseries bucket, even where totaldrop is non-zero, while
// tableData reports it as totaldrop/volume-tx, a fraction. Each bucket gets that same
// ratio here; a bucket with no transmitted volume has no defined ratio, so its percent
// drop is marked unset. fields returns pointers to the row's volume, drop, percent drop
// and unset marker, so path and circuit QoS share the computation.
func computePercentDrop[T any](metrics []T, fields func(*T) (volumeTx, totalDrop, percentDrop *float64, unset *bool)) {
	for i := range metrics {
		volumeTx, totalDrop, percentDrop, unset := fields(&metrics[i])
		if *volumeTx == 0 {
			*percentDrop = 0
			*unset = true
			continue
		}
		*percentDrop = *totalDrop / *volumeTx
		*unset = false
	}
}

// GetLinkStatusMetrics retrieves link status metrics from the Versa Analytics API
func (client *Client) GetLinkStatusMetrics(tenant string) ([]LinkStatusMetrics, error) {
	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointLinkStatusMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"linkstatus(site,accckt)",
		"",
		"",
		[]string{
			"availability",
		},
		nil,
		parseLinkStatusMetrics,
	)
}

// GetLinkUsageMetrics gets link metrics for a Versa tenant
func (client *Client) GetLinkUsageMetrics(tenant string) ([]LinkUsageMetrics, error) {
	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointLinkUsageMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"linkusage(site,accckt,accckt.uplinkBW,accckt.downlinkBW,accckt.type,accckt.media,accckt.ip,accckt.isp)",
		"",
		"",
		[]string{
			"volume-tx",
			"volume-rx",
			"bw-tx",
			"bw-rx",
		},
		nil,
		parseLinkUsageMetrics,
	)
}

// GetSiteMetrics gets site metrics for a Versa tenant
func (client *Client) GetSiteMetrics(tenant string) ([]SiteMetrics, error) {
	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointSiteMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"linkusage(site,site.address,site.latitude,site.longitude,site.locationSource)",
		"",
		"siteStatus",
		[]string{
			"volume-tx",
			"volume-rx",
			"bw-tx",
			"bw-rx",
			"availability",
		},
		nil,
		parseSiteMetrics,
	)
}

// GetApplicationsByAppliance retrieves applications by appliance metrics from the Versa Analytics API
func (client *Client) GetApplicationsByAppliance(tenant string) ([]ApplicationsByApplianceMetrics, error) {
	// TODO: should the lookback be configurable for these? no data is returned for 30min lookback
	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointApplicationsByApplianceMetrics),
		tenant,
		"SDWAN",
		"1daysAgo",
		"app(site,appId)",
		"",
		"",
		[]string{
			"sessions",
			"volume-tx",
			"volume-rx",
			"bw-tx",
			"bw-rx",
			"bandwidth",
		},
		nil,
		parseApplicationsByApplianceMetrics,
	)
}

// GetTopUsers retrieves top users of applications by appliance from the Versa Analytics API
func (client *Client) GetTopUsers(tenant string) ([]TopUserMetrics, error) {
	// TODO: should the lookback be configurable for these? no data is returned for 30min lookback
	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointTopUserMetrics),
		tenant,
		"SDWAN",
		"1daysAgo",
		"appUser(site,user)",
		"",
		"",
		[]string{
			"sessions",
			"volume-tx",
			"volume-rx",
			"bw-tx",
			"bw-rx",
			"bandwidth",
		},
		nil,
		parseTopUserMetrics,
	)
}

// GetTunnelMetrics retrieves tunnel metrics from the Versa Analytics API
func (client *Client) GetTunnelMetrics(tenant string) ([]TunnelMetrics, error) {
	if tenant == "" {
		return nil, errors.New("tenant cannot be empty")
	}

	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointTunnelMetrics),
		tenant,
		"SYSTEM",
		client.lookback,
		"tunnelstats(appliance,ipsecLocalIp,ipsecPeerIp,ipsecVpnProfName)",
		"",
		"",
		[]string{
			"volume-tx",
			"volume-rx",
		},
		nil,
		parseTunnelMetrics,
	)
}

// GetDIAMetrics retrieves DIA (Direct Internet Access) metrics from the Versa Analytics API
func (client *Client) GetDIAMetrics(tenant string) ([]DIAMetrics, error) {
	if tenant == "" {
		return nil, errors.New("tenant cannot be empty")
	}

	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointDIAMetrics),
		tenant,
		"SDWAN",
		"1daysAgo",
		"usage(site,accckt,accckt.ip)",
		"(accessType:DIA)",
		"",
		[]string{
			"volume-tx",
			"volume-rx",
			"bw-tx",
			"bw-rx",
		},
		nil,
		parseDIAMetrics,
	)
}

// GetAnalyticsInterfaces retrieves interface utilization metrics from the Versa Analytics API
func (client *Client) GetAnalyticsInterfaces(tenant string) ([]AnalyticsInterfaceMetrics, error) {
	if tenant == "" {
		return nil, errors.New("tenant cannot be empty")
	}

	return getPaginatedAnalytics(
		client,
		client.endpointConfig(EndpointInterfaceMetrics),
		tenant,
		"SDWAN",
		client.lookback,
		"intfUtil(site,accCkt,intf)",
		"",
		"",
		[]string{
			"rxUtil",
			"txUtil",
			"volume-rx",
			"volume-tx",
			"volume",
			"bw-rx",
			"bw-tx",
			"bandwidth",
		},
		nil,
		parseAnalyticsInterfaceMetrics,
	)
}

// analyticsQuery describes a single Versa Analytics query.
//
// Fields:
//   - tenant: tenant name within the environment (e.g., "datadog")
//   - feature: category of analytics metrics (e.g., "SDWAN", "SYSTEM", "CGNAT", etc.)
//   - queryType: type of query, QueryTypeTableData or QueryTypeTimeseries
//   - query: Versa query expression (e.g., "slam(...columns...)")
//   - lookback: relative start date (e.g., "15minutesAgo", "1h", "24h")
//   - endDate: relative end date, only sent for timeseries queries
//   - gap: bucket size for timeseries queries, one of "1MINUTE", "1HOUR" or "1DAY",
//     ignored otherwise. Versa rejects any other value, 15MINUTE included, and renders
//     at the tenant's configured resolution whichever of the three is asked for.
//   - filterQuery: filter query (e.g. "(accessType:DIA)")
//   - joinQuery: table to join from (e.g. "siteStatus")
//   - metrics: list of metric strings (e.g., "delay", "fwdLossRatio")
//   - count: number of rows to retrieve (similar to limit)
//   - fromCount: row to start at (similar to offset)
type analyticsQuery struct {
	tenant      string
	feature     string
	queryType   string
	query       string
	lookback    string
	endDate     string
	gap         string
	filterQuery string
	joinQuery   string
	metrics     []string
	count       int
	fromCount   int
}

// buildAnalyticsPath constructs a Versa Analytics query path, returning the full encoded
// URL string.
func buildAnalyticsPath(q analyticsQuery) string {
	baseAnalyticsPath := "/versa/analytics/v1.0.0/data/provider"
	path := fmt.Sprintf("%s/tenants/%s/features/%s", baseAnalyticsPath, q.tenant, q.feature)
	params := url.Values{
		"start-date": []string{q.lookback},
		"qt":         []string{q.queryType},
		"q":          []string{q.query},
		"ds":         []string{"aggregate"}, // this seems to be the only datastore supported (from docs)
		"count":      []string{strconv.Itoa(q.count)},
		"from-count": []string{strconv.Itoa(q.fromCount)},
	}
	// endDate and gap only apply to timeseries queries, and sending them on a tableData
	// query would change requests that work today, so both stay opt-in.
	if q.endDate != "" {
		params.Add("end-date", q.endDate)
	}
	if q.gap != "" {
		params.Add("gap", q.gap)
	}
	// filterQuery is not required for most calls
	// only include in the params if needed
	if q.filterQuery != "" {
		params.Add("fq", q.filterQuery)
	}
	// joinQuery is not required for most calls
	// only include in the params if needed
	if q.joinQuery != "" {
		params.Add("jq", q.joinQuery)
	}
	for _, m := range q.metrics {
		params.Add("metrics", m)
	}
	return path + "?" + params.Encode()
}

func createLookbackString(lookbackMinutes int) string {
	return fmt.Sprintf("%dminutesAgo", lookbackMinutes)
}
