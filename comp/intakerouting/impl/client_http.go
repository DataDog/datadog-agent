// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// The wire format below is provisional. Replace it when the backend contract is final.
//
// Request:
//
//	GET https://config.<site>/api/unstable/agent/intake-routing
//	DD-API-KEY: <key>
//
// Response (HTTP 200):
//
//	{"routes": {"logs": {"override": true, "host": "<host>", "port": 443}}}
//
// The request does not select telemetry types: the response holds an entry for every type the
// backend has an answer for. "override": false means "no override": use the default endpoint.
// Unknown telemetry types are ignored. HTTP 404 means the backend does not offer routing.
//
// The pre-flight check is a GET of intake_routing.preflight_path (/api/v1/validate by default) on
// the override host. Like the queries, it sends the API key in the DD-API-KEY header, as the
// connectivity diagnosis does for /api/v1/validate: never in the URL, where proxies and access logs
// could record it.
const (
	routingURLPrefix = "https://config."
	routingPath      = "/api/unstable/agent/intake-routing"

	apiKeyHeader = "DD-API-KEY"

	// maxResponseBytes bounds how much of a response body is read.
	maxResponseBytes = 64 << 10
	// maxQueryRedirects bounds how many redirects a query follows.
	maxQueryRedirects = 3
)

var errRedirectRefused = errors.New("redirect refused")

type wireResponse struct {
	Routes map[string]wireRoute `json:"routes"`
}

type wireRoute struct {
	Override bool   `json:"override"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

// httpRoutingClient implements routingClient over HTTPS.
type httpRoutingClient struct {
	queryClient     *http.Client
	preflightClient *http.Client
	// baseURL returns the scheme and host of the routing API of a site.
	baseURL func(site string) string
	// preflightPath is the escaped path of the pre-flight check, as returned by parsePreflightPath.
	preflightPath string
	userAgent     string
}

// newHTTPRoutingClientWithTransport returns a client that sends its requests with transport.
// preflightPath must come from parsePreflightPath.
func newHTTPRoutingClientWithTransport(transport http.RoundTripper, preflightPath string) *httpRoutingClient {
	return &httpRoutingClient{
		queryClient: &http.Client{
			Transport:     transport,
			CheckRedirect: checkQueryRedirect,
		},
		preflightClient: &http.Client{
			Transport: transport,
			// A redirect fails the pre-flight check: the 3xx response is returned as is.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		baseURL: func(site string) string {
			return pkgconfigutils.BuildURLWithPrefix(routingURLPrefix, site)
		},
		preflightPath: preflightPath,
		userAgent:     "datadog-agent/" + version.AgentVersion,
	}
}

// parsePreflightPath checks that p is an absolute URL path, with no scheme, host, query or
// fragment, and returns it in its escaped form. The host of a pre-flight check must be the host
// of the override, so p must not be able to change it.
func parsePreflightPath(p string) (string, error) {
	if strings.TrimSpace(p) != p {
		return "", errors.New("it starts or ends with a space")
	}
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "", errors.New("it is not an absolute path")
	}
	if strings.ContainsAny(p, "?#") {
		return "", errors.New("it has a query or a fragment")
	}
	u, err := url.Parse(p)
	if err != nil {
		return "", err
	}
	return u.EscapedPath(), nil
}

// checkQueryRedirect only follows a few redirects, each to an HTTPS URL of an official Datadog
// domain: Go forwards the API key header to the redirect target.
func checkQueryRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxQueryRedirects {
		return fmt.Errorf("%w: more than %d redirects", errRedirectRefused, maxQueryRedirects)
	}
	if req.URL.Scheme != "https" || !pkgconfigutils.IsDatadogURL(req.URL.String()) {
		return fmt.Errorf("%w: %s is not an HTTPS URL of an official Datadog domain", errRedirectRefused, req.URL.Redacted())
	}
	return nil
}

func (c *httpRoutingClient) query(ctx context.Context, site, apiKey string) queryResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL(site)+routingPath, nil)
	if err != nil {
		return queryResult{status: statusNetworkError, err: err}
	}
	req.Header.Set(apiKeyHeader, apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.queryClient.Do(req)
	if err != nil {
		return queryResult{status: transportErrorStatus(err), err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		drain(resp.Body)
		return queryResult{status: statusNotOffered, code: resp.StatusCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		drain(resp.Body)
		return queryResult{status: statusHTTPError, code: resp.StatusCode, err: fmt.Errorf("unexpected HTTP status %q", resp.Status)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return queryResult{status: transportErrorStatus(err), code: resp.StatusCode, err: err}
	}
	if len(body) > maxResponseBytes {
		return queryResult{status: statusInvalidResponse, code: resp.StatusCode, err: fmt.Errorf("response is larger than %d bytes", maxResponseBytes)}
	}
	routes, err := parseRoutingResponse(body)
	if err != nil {
		return queryResult{status: statusInvalidResponse, code: resp.StatusCode, err: err}
	}
	return queryResult{status: statusSuccess, code: resp.StatusCode, routes: routes}
}

func parseRoutingResponse(body []byte) (map[intakerouting.TelemetryType]routeAnswer, error) {
	var resp wireResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}
	if resp.Routes == nil {
		return nil, errors.New(`response has no "routes" object`)
	}
	routes := make(map[intakerouting.TelemetryType]routeAnswer, len(resp.Routes))
	for name, route := range resp.Routes {
		routes[intakerouting.TelemetryType(name)] = routeAnswer{
			override: route.Override,
			host:     route.Host,
			port:     route.Port,
		}
	}
	return routes, nil
}

func (c *httpRoutingClient) preflight(ctx context.Context, ep intakerouting.Endpoint, apiKey string) error {
	// The path is already escaped: it must not go through url.URL.Path, which would escape it again.
	u := "https://" + hostPort(ep) + c.preflightPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set(apiKeyHeader, apiKey)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.preflightClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	drain(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("unexpected HTTP status %q", resp.Status)
	}
	return nil
}

// transportErrorStatus maps an error returned by an HTTP client to a query status.
func transportErrorStatus(err error) string {
	if errors.Is(err, errRedirectRefused) {
		return statusHTTPError
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return statusTimeout
	}
	return statusNetworkError
}

// drain reads what is left of a body, up to maxResponseBytes, so the connection can be reused.
func drain(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxResponseBytes))
}

func hostPort(ep intakerouting.Endpoint) string {
	if ep.Port == 0 {
		return ep.Host
	}
	return net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port))
}
