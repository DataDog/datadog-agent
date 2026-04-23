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
	defaultBasicPort   = 9182
	defaultOAuthPort   = 9183
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
	maxCount            string // Stored as string to be passed as an HTTP param
	lookback            string
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
		authMethod:          authMethod,
		username:            authConfig.Username,
		password:            authConfig.Password,
		clientID:            authConfig.ClientID,
		clientSecret:        authConfig.ClientSecret,
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
		params["offset"] = strconv.Itoa(i * maxCount)
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
		return nil, errors.New("tenantName cannot be empty")
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

// GetTopology retrieves topology data for for a specific appliance and tenant
func (client *Client) GetTopology(applianceName string) ([]Neighbor, error) {

	if applianceName == "" {
		return nil, fmt.Errorf("applianceName cannot be empty")
	}
	// if tenantName == "" {
	// 	return nil, fmt.Errorf("tenantName cannot be empty")
	// }

	// params := map[string]string{
	// 	"tenantName": tenantName,
	// }

	// command := 'lldp/neighbor/detail/interface-detail'
	// path := fmt.Sprintf("/vnms/dashboard/appliance/%s/live?uuid=%s&command%s", applianceName, UUID, command)

	// resp, err := get[LldpNeighborInterfaceDetailResponse](client, path, params, false)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get topology: %v", err)
	// }
	// if resp == nil {
	// 	return nil, errors.New("failed to get topology: returned nil")
	// }

	return []Neighbor{
		Neighbor{
			SystemName: "Mock System Name",
			SystemDescription: "Mock System Description",
			ChassisID: "1",
			DeviceIDType: "chassisID",
			IPAddress: "10.0.0.1",
			PortID: "1",
			PortIDType: "portID",
			PortDescription: "Port Description 1",
		},
		Neighbor{
			SystemName: "Mock System Name 2",
			SystemDescription: "Mock System Description 2",
			ChassisID: "2",
			DeviceIDType: "chassisID",
			IPAddress: "10.0.0.2",
			PortID: "2",
			PortIDType: "portID",
			PortDescription: "Port Description 2",
		},
		Neighbor{
			SystemName: "Mock System Name 3",
			SystemDescription: "Mock System Description 3",
			ChassisID: "3",
			DeviceIDType: "chassisID",
			IPAddress: "10.0.0.3",
			PortID: "3",
			PortIDType: "portID",
			PortDescription: "Port Description 3",
		},
	}, nil
}

// GetInterfaceMetrics retrieves interface metrics for a specific appliance and tenant using pagination
func (client *Client) GetInterfaceMetrics(applianceName string, tenantName string) ([]InterfaceMetrics, error) {
	if applianceName == "" {
		return nil, errors.New("applianceName cannot be empty")
	}
	if tenantName == "" {
		return nil, errors.New("tenantName cannot be empty")
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

// GetSLAMetrics retrieves SLA metrics from the Versa Analytics API
func (client *Client) GetSLAMetrics(tenant string) ([]SLAMetrics, error) {
	return getPaginatedAnalytics(
		client,
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
		parseSLAMetrics,
	)
}

// GetPathQoSMetrics retrieves QoS (Class of Service) metrics from the Versa Analytics API
func (client *Client) GetPathQoSMetrics(tenant string) ([]QoSMetrics, error) {
	return getPaginatedAnalytics(
		client,
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
		parsePathQoSMetrics,
	)
}

// GetLinkStatusMetrics retrieves link status metrics from the Versa Analytics API
func (client *Client) GetLinkStatusMetrics(tenant string) ([]LinkStatusMetrics, error) {
	return getPaginatedAnalytics(
		client,
		tenant,
		"SDWAN",
		client.lookback,
		"linkstatus(site,accckt)",
		"",
		"",
		[]string{
			"availability",
		},
		parseLinkStatusMetrics,
	)
}

// GetLinkUsageMetrics gets link metrics for a Versa tenant
func (client *Client) GetLinkUsageMetrics(tenant string) ([]LinkUsageMetrics, error) {
	return getPaginatedAnalytics(
		client,
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
		parseLinkUsageMetrics,
	)
}

// GetSiteMetrics gets site metrics for a Versa tenant
func (client *Client) GetSiteMetrics(tenant string) ([]SiteMetrics, error) {
	return getPaginatedAnalytics(
		client,
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
		parseSiteMetrics,
	)
}

// GetApplicationsByAppliance retrieves applications by appliance metrics from the Versa Analytics API
func (client *Client) GetApplicationsByAppliance(tenant string) ([]ApplicationsByApplianceMetrics, error) {
	// TODO: should the lookback be configurable for these? no data is returned for 30min lookback
	return getPaginatedAnalytics(
		client,
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
		parseApplicationsByApplianceMetrics,
	)
}

// GetTopUsers retrieves top users of applications by appliance from the Versa Analytics API
func (client *Client) GetTopUsers(tenant string) ([]TopUserMetrics, error) {
	// TODO: should the lookback be configurable for these? no data is returned for 30min lookback
	return getPaginatedAnalytics(
		client,
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
		parseAnalyticsInterfaceMetrics,
	)
}

// buildAnalyticsPath constructs a Versa Analytics query path in a cleaner way so multiple metrics can be added.
// TODO: maybe this becomes a struct function. Modifications will be easier and the function signature will be
// much cleaner
//
// Parameters:
//   - tenant: tenant name within the environment (e.g., "datadog")
//   - feature: category of analytics metrics (e.g., "SDWAN, "SYSTEM", "CGNAT", etc.)
//   - lookback: relative start date (e.g., "15minutesAgo", "1h", "24h")
//   - query: Versa query expression (e.g., "slam(...columns...)")
//   - queryType: type of query (e.g., "tableData", "table", "summary")
//   - filterQuery: filter query (e.g. "(accessType:DIA)")
//   - joinQuery: table to join from (e.g. "siteStatus")
//   - metrics: list of metric strings (e.g., "delay", "fwdLossRatio")
//   - count: number of rows to retrieve (similar to limit)
//   - fromCount: row to start at (similar to offset)
//
// Returns the full encoded URL string.
func buildAnalyticsPath(tenant string, feature string, lookback string, query string, queryType string, filterQuery string, joinQuery string, metrics []string, count int, fromCount int) string {
	baseAnalyticsPath := "/versa/analytics/v1.0.0/data/provider"
	path := fmt.Sprintf("%s/tenants/%s/features/%s", baseAnalyticsPath, tenant, feature)
	params := url.Values{
		"start-date": []string{lookback},
		"qt":         []string{queryType},
		"q":          []string{query},
		"ds":         []string{"aggregate"}, // this seems to be the only datastore supported (from docs)
		"count":      []string{strconv.Itoa(count)},
		"from-count": []string{strconv.Itoa(fromCount)},
	}
	// filterQuery is not required for most calls
	// only include in the params if needed
	if filterQuery != "" {
		params.Add("fq", filterQuery)
	}
	// joinQuery is not required for most calls
	// only include in the params if needed
	if joinQuery != "" {
		params.Add("jq", joinQuery)
	}
	for _, m := range metrics {
		params.Add("metrics", m)
	}
	return path + "?" + params.Encode()
}

func createLookbackString(lookbackMinutes int) string {
	return fmt.Sprintf("%dminutesAgo", lookbackMinutes)
}
