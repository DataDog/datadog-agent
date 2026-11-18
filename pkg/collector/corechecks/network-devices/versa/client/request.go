// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
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
	return nil, &requestError{
		endpoint:   endpoint,
		statusCode: statusCode,
		cause:      lastErr,
		apiMessage: apiErrorMessage(bytes),
		apiError:   isAPIErrorBody(bytes),
	}
}

// requestError is a request that did not get a valid response after every attempt. Its
// message is unchanged from before it carried the status and Versa's error message.
type requestError struct {
	endpoint   string
	statusCode int
	cause      error
	// apiMessage is the "error" field of a JSON error body, truncated, or empty.
	apiMessage string
	// apiError is true when the body is a Versa JSON error object with a code or error field.
	apiError bool
}

func (e *requestError) Error() string {
	return fmt.Sprintf("%s http responded with %d code and error %v", e.endpoint, e.statusCode, e.cause)
}

// maxAPIMessageLength bounds the Versa error message carried into log lines.
const maxAPIMessageLength = 200

// apiErrorMessage returns the "error" field of a Versa JSON error body, truncated, or an
// empty string when the body is not such an object.
func apiErrorMessage(body []byte) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	if len(parsed.Error) > maxAPIMessageLength {
		return parsed.Error[:maxAPIMessageLength] + "..."
	}
	return parsed.Error
}

// isAPIErrorBody reports whether body is a Versa JSON error object, carrying a code or
// an error field.
func isAPIErrorBody(body []byte) bool {
	var parsed map[string]json.RawMessage
	if json.Unmarshal(body, &parsed) != nil {
		return false
	}
	_, hasCode := parsed["code"]
	_, hasError := parsed["error"]
	return hasCode || hasError
}

// timeseriesRejection returns the HTTP status and Versa error message when err is Versa
// rejecting the request itself: a 4xx other than 401 and 403, with a Versa JSON error
// body. Authentication failures, HTML responses (reported as 401), transport errors,
// timeouts and 5xx responses are not rejections and keep their usual error handling.
func timeseriesRejection(err error) (int, string, bool) {
	var reqErr *requestError
	if !errors.As(err, &reqErr) {
		return 0, "", false
	}
	status := reqErr.statusCode
	if status < 400 || status >= 500 || status == 401 || status == 403 || !reqErr.apiError {
		return 0, "", false
	}
	return status, reqErr.apiMessage, true
}

// isBodyParseError reports whether err is a response body that is not valid JSON for the
// expected type.
func isBodyParseError(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
}

// maxLoggedMessageLength caps a sanitized Versa error message in a log line.
const maxLoggedMessageLength = 120

var (
	// loggedMessageDrop matches tokens that could carry request details: URLs, paths,
	// query strings and host:port pairs.
	loggedMessageDrop = regexp.MustCompile(`\S*(://|/|\?|&|=)\S*|\S*[A-Za-z0-9.-]+:\d+\S*`)
	whitespace        = regexp.MustCompile(`\s+`)
)

// sanitizeAPIMessage makes a Versa error message safe for a log line: tokens that look
// like a URL, path, query string or host:port are removed, values the client sent
// (credentials, session tokens, tenant) are redacted, whitespace is collapsed and the
// result is capped at maxLoggedMessageLength characters.
func (client *Client) sanitizeAPIMessage(message string, tenant string) string {
	message = loggedMessageDrop.ReplaceAllString(message, "")
	for _, secret := range []string{client.password, client.username, client.clientSecret, client.sessionToken, client.directorToken, tenant} {
		if len(secret) >= 3 {
			message = strings.ReplaceAll(message, secret, "<redacted>")
		}
	}
	message = strings.TrimSpace(whitespace.ReplaceAllString(message, " "))
	if len(message) > maxLoggedMessageLength {
		message = message[:maxLoggedMessageLength]
	}
	return message
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

	return getTableDataAnalytics(client, cfg, tenant, feature, lookback, query, filterQuery, joinQuery, metrics, parser)
}

// getTableDataAnalytics runs a tableData query, paging through the results.
func getTableDataAnalytics[T any](
	client *Client,
	cfg EndpointConfig,
	tenant string,
	feature string,
	lookback string,
	query string,
	filterQuery string,
	joinQuery string,
	metrics []string,
	parser func([][]interface{}) ([]T, error),
) ([]T, error) {
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

// fetchTableDataRows returns the raw rows of a tableData query, paging through the
// results, for joining attribute columns onto a timeseries response.
func fetchTableDataRows(
	client *Client,
	cfg EndpointConfig,
	tenant string,
	feature string,
	lookback string,
	query string,
	filterQuery string,
	joinQuery string,
	metrics []string,
) ([][]interface{}, error) {
	return getTableDataAnalytics(client, cfg, tenant, feature, lookback, query, filterQuery, joinQuery, metrics,
		func(rows [][]interface{}) ([][]interface{}, error) { return rows, nil })
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
	// rows, and how count and from-count apply to it (by entity, by series or not at all)
	// is still to be confirmed against a customer Director. Paging on
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

	// useTableData answers the call from the tableData request for the same query, as
	// it would be answered with analytics_query_type set to tableData, and logs why.
	useTableData := func(code FallbackReason, reason string, series []AnalyticsTimeseries) ([]T, error) {
		client.recordFallback(tenant, query, code)
		client.warnRateLimited("fallback:"+query+":"+string(code), "Timeseries query %s: %s, using tableData for it instead", shortQueryName(query), reason)
		tableRows, err := fetchTableDataRows(client, cfg, tenant, feature, lookback, query, filterQuery, joinQuery, metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to get analytics metrics: %v", err)
		}
		client.debugSamples(query, series, tableRows)
		parsed, err := parser(tableRows)
		if err != nil {
			return nil, fmt.Errorf("failed to parse analytics metrics: %v", err)
		}
		return parsed, nil
	}

	resp, err := get[AnalyticsTimeseriesResponse](client, analyticsURL, nil, true, cfg.Timeout)
	if err != nil {
		if status, message, ok := timeseriesRejection(err); ok {
			reason := fmt.Sprintf("Versa rejected the timeseries request with HTTP %d", status)
			if message = client.sanitizeAPIMessage(message, tenant); message != "" {
				reason += ": " + message
			}
			return useTableData(FallbackHTTPError, reason, nil)
		}
		if isBodyParseError(err) {
			return useTableData(FallbackHTTPError, "the timeseries response could not be parsed", nil)
		}
		return nil, fmt.Errorf("failed to get analytics timeseries: %v", err)
	}
	log.Debugf("Timeseries query %q returned %d series for %d entities", query, len(resp.Data), countEntities(resp.Data))

	if layout.strictKeyParts {
		if name, mismatch := keyPartsMismatch(resp.Data, layout); mismatch {
			return useTableData(FallbackKeyParts, fmt.Sprintf("an entity key has %d parts where %d were expected", len(strings.Split(name, ",")), layout.drillKeyFields), resp.Data)
		}
	}

	if entities := countEntities(resp.Data); entities >= cfg.MaxCount {
		log.Warnf("Timeseries query %q returned %d entities, at or above max_count %d: raise max_count, results may be truncated", query, entities, cfg.MaxCount)
	}

	// The parser reads metric values by position; a layout may list them in the parser's
	// order when that differs from the requested order, so each named series lands in the
	// field of the same name.
	valueOrder := metrics
	if layout.valueOrder != nil {
		valueOrder = layout.valueOrder
	}
	rows, dropped := flattenTimeseries(resp.Data, valueOrder, layout.drillKeyFields)
	if dropped > 0 {
		if len(rows) == 0 {
			// Every bucket was unusable. Most likely a requested metric is not returned
			// as a series on this tenant, and the endpoint reports nothing at all.
			log.Warnf("Dropped all %d timeseries buckets for query %q, no metrics will be reported for it", dropped, query)
		} else {
			log.Debugf("Dropped %d incomplete timeseries buckets for query %q", dropped, query)
		}
	}

	if layout.attributeColumns > 0 && len(rows) > 0 {
		tableRows, err := fetchTableDataRows(client, cfg, tenant, feature, lookback, query, filterQuery, joinQuery, metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to get analytics tableData for timeseries attributes: %v", err)
		}
		entities := distinctRowKeys(rows)
		var missing, ambiguousKeys int
		rows, missing, ambiguousKeys = joinAttributes(rows, tableRows, layout)
		if ambiguousKeys > 0 {
			// The timeseries response cannot tell apart the entities behind an ambiguous
			// key, so report exactly what tableData reports for this call.
			return fallBackFromJoin(client, tenant, query, FallbackJoinAmbiguous, fmt.Sprintf("%d entity keys match tableData rows with differing attributes", ambiguousKeys), resp.Data, tableRows, parser)
		}
		matched := distinctRowKeys(rows)
		if unmatched := entities - matched; matched == 0 || unmatched > matched {
			// The timeseries and tableData keys mostly disagree, so joining would drop
			// most of the family. Report what tableData reports instead.
			return fallBackFromJoin(client, tenant, query, FallbackJoinUnmatched, fmt.Sprintf("only %d of %d timeseries entities match a tableData row", matched, entities), resp.Data, tableRows, parser)
		}
		if missing > 0 {
			client.warnRateLimited("join:"+query, "Skipped %d timeseries buckets for query %q whose entity has no tableData row to take its attributes from", missing, query)
		}
	}

	rawRows := make([][]interface{}, 0, len(rows))
	for _, row := range rows {
		rawRows = append(rawRows, row.row)
	}

	parsed, err := parser(rawRows)
	if err != nil {
		return useTableData(FallbackHTTPError, "the timeseries response could not be parsed", resp.Data)
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

// fallBackFromJoin answers a join endpoint's call from the tableData rows already fetched
// for the join, and logs why.
func fallBackFromJoin[T any](client *Client, tenant, query string, code FallbackReason, reason string, series []AnalyticsTimeseries, tableRows [][]interface{}, parser func([][]interface{}) ([]T, error)) ([]T, error) {
	client.recordFallback(tenant, query, code)
	client.warnRateLimited("fallback:"+query+":"+string(code), "Timeseries query %s: %s, using tableData for it instead", shortQueryName(query), reason)
	client.debugSamples(query, series, tableRows)
	parsed, err := parser(tableRows)
	if err != nil {
		return nil, fmt.Errorf("failed to parse analytics tableData: %v", err)
	}
	return parsed, nil
}

// maxNameSamples is how many timeseries series names and tableData keys a fallback logs.
const maxNameSamples = 3

// debugSamples logs, at debug level and at most once an hour per query, up to
// maxNameSamples distinct timeseries series names and tableData row keys with their part
// counts, so a debug log shows how the two responses name entities. These are Versa
// entity names (sites, circuits, IPs), never request details or credentials.
func (client *Client) debugSamples(query string, series []AnalyticsTimeseries, tableRows [][]interface{}) {
	var names, keys []string
	seen := make(map[string]bool)
	for _, s := range series {
		if len(names) == maxNameSamples {
			break
		}
		if !seen[s.Name] {
			seen[s.Name] = true
			names = append(names, fmt.Sprintf("%q (%d parts)", s.Name, len(strings.Split(s.Name, ","))))
		}
	}
	for _, row := range tableRows {
		if len(keys) == maxNameSamples {
			break
		}
		if len(row) == 0 {
			continue
		}
		if key, ok := row[0].(string); ok {
			keys = append(keys, fmt.Sprintf("%q (%d parts)", key, len(strings.Split(key, ","))))
		}
	}
	client.limiter().Debugf("samples:"+query, "Timeseries query %q fell back to tableData; timeseries series names: [%s]; tableData keys: [%s]", query, strings.Join(names, ", "), strings.Join(keys, ", "))
}

// distinctRowKeys returns the number of distinct entity keys (column zero) across rows.
func distinctRowKeys(rows []timeseriesRow) int {
	keys := make(map[interface{}]struct{}, len(rows))
	for _, r := range rows {
		keys[r.row[0]] = struct{}{}
	}
	return len(keys)
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
