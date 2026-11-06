// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader, useSessionAuth bool) (*http.Request, error) {
	// session auth requires token authentication
	if useSessionAuth {
		return http.NewRequestWithContext(context.Background(), method, client.directorEndpoint+uri, body)
	}
	log.Tracef("Endpoint: %s:%d%s", client.directorEndpoint, client.directorAPIPort, uri)
	return http.NewRequestWithContext(context.Background(), method, fmt.Sprintf("%s:%d%s", client.directorEndpoint, client.directorAPIPort, uri), body)
}

// requestTarget renders the request target for logging: the path plus the query string
// when there is one. Only the URL is rendered, never headers or the request body, so
// that credentials sent by the authentication calls are not written to the log.
func requestTarget(req *http.Request) string {
	if req.URL.RawQuery == "" {
		return req.URL.Path
	}
	return req.URL.Path + "?" + req.URL.RawQuery
}

// executeRequest performs an HTTP request and logs it at debug level. Every Versa API call
// the Agent makes goes through here, so enabling debug logs yields one line per call with
// the method, path, query parameters, response code, and how long it took.
func (client *Client) executeRequest(req *http.Request, httpClient *http.Client) (*http.Response, error) {
	start := timeNow()
	resp, err := httpClient.Do(req)
	elapsed := timeNow().Sub(start).Round(time.Millisecond)

	if err != nil {
		log.Debugf("Versa API call %s %s failed after %s: %v", req.Method, requestTarget(req), elapsed, err)
		return nil, err
	}

	log.Debugf("Versa API call %s %s responded %d in %s", req.Method, requestTarget(req), resp.StatusCode, elapsed)
	return resp, nil
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// do exec a request with authentication
func (client *Client) do(req *http.Request, timeout time.Duration) ([]byte, int, error) {
	httpClient := client.httpClient
	if timeout > 0 && timeout != httpClient.Timeout {
		// shallow copy shares the transport (TLS config, connection pool) and cookie jar
		withTimeout := *httpClient
		withTimeout.Timeout = timeout
		httpClient = &withTimeout
	}
	resp, err := client.executeRequest(req, httpClient)
	if err != nil {
		return nil, 0, err
	}

	defer resp.Body.Close()

	// TODO: should we bring this back with OAuth?
	if !isAuthenticated(resp.Header) {
		log.Debugf("Versa API call %s %s returned an HTML response, treating it as unauthenticated", req.Method, requestTarget(req))
		// clear auth to trigger re-authentication
		client.clearAuth()
		// Return 401 on auth errors
		return nil, 401, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return body, resp.StatusCode, nil
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use similar
// get implementations to execute GET requests to the given endpoint with the given query params
// timeout applies to each attempt
func (client *Client) get(endpoint string, params map[string]string, useSessionAuth bool, timeout time.Duration) ([]byte, error) {
	var bytes []byte
	var statusCode int
	var lastErr error

	for attempts := 0; attempts < client.maxAttempts; attempts++ {
		if useSessionAuth {
			// Always authenticate session for Analytics endpoints
			err := client.authenticateSession()
			if err != nil {
				return nil, err
			}
		} else {
			// Authenticate Director API (OAuth needs pre-auth, Basic doesn't)
			err := client.authenticateDirector()
			if err != nil {
				return nil, err
			}
		}

		// Now create the request with proper authentication
		req, err := client.newRequest("GET", endpoint, nil, useSessionAuth)
		if err != nil {
			return nil, err
		}

		// Set authentication headers AFTER authentication
		if useSessionAuth {
			// use session token for Analytics endpoints
			req.Header.Add("X-CSRF-TOKEN", client.sessionToken)
		} else {
			switch client.authMethod {
			case authMethodOAuth:
				// use OAuth Bearer token for Director API endpoints
				req.Header.Add("Authorization", "Bearer "+client.directorToken)
			case authMethodBasic:
				// use HTTP Basic authentication for Director API endpoints
				req.SetBasicAuth(client.username, client.password)
			default:
				return nil, fmt.Errorf("unsupported authentication method: %s", client.authMethod)
			}
		}

		query := req.URL.Query()
		for key, value := range params {
			query.Add(key, value)
		}
		req.URL.RawQuery = query.Encode()

		bytes, statusCode, err = client.do(req, timeout)

		if err == nil && isValidStatusCode(statusCode) {
			// Got a valid response, stop retrying
			return bytes, nil
		}
		lastErr = err
	}

	log.Tracef("%d error code hitting endpoint %q with error %+v and response: %s", statusCode, endpoint, lastErr, string(bytes))
	return nil, fmt.Errorf("%s http responded with %d code and error %v", endpoint, statusCode, lastErr)
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
// get wraps client.get with generic type content and unmarshalling (methods can't use generics)
func get[T Content](client *Client, endpoint string, params map[string]string, useSessionAuth bool, timeout time.Duration) (*T, error) {
	bytes, err := client.get(endpoint, params, useSessionAuth, timeout)
	if err != nil {
		return nil, err
	}

	var data T

	err = json.Unmarshal(bytes, &data)
	if err != nil {
		log.Tracef("Failed to unmarshal response: %s", err)
		// Log the response body for debugging
		log.Tracef("Response body: %s", string(bytes))
		return nil, err
	}

	return &data, nil
}

func isValidStatusCode(code int) bool {
	return code >= 200 && code < 400
}

// getPaginatedAnalytics handles the common pagination pattern for all analytics endpoints
// TODO: perhaps this can be a struct? that way there's no passing around of arguments through
// layers of stuff
func getPaginatedAnalytics[T any](
	client *Client,
	cfg EndpointConfig,
	tenant string,
	feature string,
	lookback string,
	query string,
	filterQuery string,
	joinQuery string,
	metrics []string,
	layout *timeseriesLayout,
	parser func([][]interface{}) ([]T, error),
) ([]T, error) {
	// An endpoint without a layout has not been verified against a real timeseries
	// response, so it stays on tableData even when the instance asks for timeseries.
	// Silently reshaping its rows would either mis-index the parser or drop every row.
	if client.analyticsQueryType == QueryTypeTimeseries {
		if layout == nil {
			log.Debugf("Query %q does not support timeseries collection, using tableData", query)
		} else {
			return getTimeseriesAnalytics(client, cfg, tenant, feature, lookback, query, filterQuery, joinQuery, metrics, layout, parser)
		}
	}

	var allMetrics []T

	// Paginate through the results
	for page := 0; page < cfg.MaxPages; page++ {
		fromCount := page * cfg.MaxCount
		analyticsURL := buildAnalyticsPath(analyticsQuery{
			tenant:      tenant,
			feature:     feature,
			queryType:   QueryTypeTableData,
			query:       query,
			lookback:    lookback,
			filterQuery: filterQuery,
			joinQuery:   joinQuery,
			metrics:     metrics,
			count:       cfg.MaxCount,
			fromCount:   fromCount,
		})

		resp, err := get[AnalyticsMetricsResponse](client, analyticsURL, nil, true, cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to get analytics metrics page %d: %v", page+1, err)
		}

		metrics, err := parser(resp.AaData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse analytics metrics page %d: %v", page+1, err)
		}

		allMetrics = append(allMetrics, metrics...)

		// If we got fewer results than maxCount, we've reached the end
		if len(metrics) < cfg.MaxCount {
			break
		}
	}

	return allMetrics, nil
}

// getTimeseriesAnalytics runs the same query as getPaginatedAnalytics with qt=timeseries,
// which returns a point per time bucket per entity instead of one aggregate row covering
// the whole lookback window. The response is flattened back into tableData shaped rows so
// the endpoint's existing parser is reused unchanged, and each parsed entry is stamped
// with its bucket time when the metric type has somewhere to put it.
func getTimeseriesAnalytics[T any](
	client *Client,
	cfg EndpointConfig,
	tenant string,
	feature string,
	lookback string,
	query string,
	filterQuery string,
	joinQuery string,
	metrics []string,
	layout *timeseriesLayout,
	parser func([][]interface{}) ([]T, error),
) ([]T, error) {
	// Deliberately a single request. A timeseries body is a list of series rather than of
	// rows, and whether count and from-count slice it by entity, by series or not at all
	// is not something we have been able to pin down against a real Director. Paging on
	// the wrong unit would either silently truncate the fleet or re-fetch and re-submit
	// the same buckets on every page, so instead we ask for everything once and say so
	// when the response looks like it was capped.
	analyticsURL := buildAnalyticsPath(analyticsQuery{
		tenant:      tenant,
		feature:     feature,
		queryType:   QueryTypeTimeseries,
		query:       query,
		lookback:    lookback,
		endDate:     timeseriesEndDate,
		gap:         client.analyticsGap,
		filterQuery: filterQuery,
		joinQuery:   joinQuery,
		metrics:     metrics,
		count:       cfg.MaxCount,
		fromCount:   0,
	})

	resp, err := get[AnalyticsTimeseriesResponse](client, analyticsURL, nil, true, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics timeseries: %v", err)
	}

	if entities := countEntities(resp.Data); entities >= cfg.MaxCount {
		log.Warnf("Timeseries query %q returned %d entities, at or above max_count %d: raise max_count, results may be truncated", query, entities, cfg.MaxCount)
	}

	rows, dropped := flattenTimeseries(resp.Data, metrics, layout.drillKeyFields)
	if dropped > 0 {
		if len(rows) == 0 {
			// Every bucket was unusable. Most likely a requested metric is not returned
			// as a series on this tenant, or the drill key does not split the way the
			// layout expects, and either way the endpoint reports nothing at all.
			log.Warnf("Dropped all %d timeseries buckets for query %q, no metrics will be reported for it", dropped, query)
		} else {
			log.Debugf("Dropped %d incomplete timeseries buckets for query %q", dropped, query)
		}
	}

	rawRows := make([][]interface{}, 0, len(rows))
	for _, row := range rows {
		rawRows = append(rawRows, row.row)
	}

	parsed, err := parser(rawRows)
	if err != nil {
		return nil, fmt.Errorf("failed to parse analytics timeseries: %v", err)
	}
	// flattenTimeseries and the parsers are both order preserving and all or nothing, so
	// parsed[i] is the entry built from rows[i].
	if len(parsed) != len(rows) {
		return nil, fmt.Errorf("parsed %d entries from %d timeseries buckets", len(parsed), len(rows))
	}
	for i := range parsed {
		if entry, ok := any(&parsed[i]).(timestamped); ok {
			entry.setTimestamp(rows[i].timestamp)
		}
	}

	return parsed, nil
}

// countEntities returns the number of distinct entities across the series of a timeseries
// response.
func countEntities(series []AnalyticsTimeseries) int {
	names := make(map[string]struct{}, len(series))
	for _, s := range series {
		names[s.Name] = struct{}{}
	}
	return len(names)
}
