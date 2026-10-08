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
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client/middleware"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	baseRetryBackoff = 1 * time.Second
	maxRetryBackoff  = 30 * time.Second
)

var newRetryBackOff = func() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = baseRetryBackoff
	b.Multiplier = 2
	// MaxInterval caps the interval before jitter is applied: leave room for the
	// randomization so jittered waits stay spread out below maxRetryBackoff
	b.MaxInterval = time.Duration(float64(maxRetryBackoff) / (1 + b.RandomizationFactor))
	return &cappedBackOff{BackOff: b, max: maxRetryBackoff}
}

// cappedBackOff clamps the waits returned by a backoff policy to max
type cappedBackOff struct {
	backoff.BackOff
	max time.Duration
}

// NextBackOff returns the wrapped policy's next wait, at most max. backoff.Stop is negative
// so it is preserved.
func (b *cappedBackOff) NextBackOff() time.Duration {
	return min(b.BackOff.NextBackOff(), b.max)
}

// newRequest creates a new request for this client.
func (client *Client) newRequest(ctx context.Context, method, uri string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, client.endpoint+uri, body)
}

// do exec a request with authentication
func (client *Client) do(req *http.Request) ([]byte, int, http.Header, error) {
	// Cross-forgery token
	client.authenticationMutex.Lock()
	req.Header.Add("X-XSRF-TOKEN", client.token)
	client.authenticationMutex.Unlock()

	log.Tracef("Executing cisco sd-wan api request %s %s", req.Method, req.URL.Path)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	log.Tracef("Executed cisco sd-wan api request %d %s %s", resp.StatusCode, req.Method, req.URL.Path)

	defer resp.Body.Close()

	if !isAuthenticated(resp.Header) {
		log.Tracef("Cisco sd-wan api request responded with invalid auth %s %s", req.Method, req.URL.Path)
		// clear auth to trigger re-authentication
		client.clearAuth()
		// Return 401 on auth errors
		return nil, 401, nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}

	return body, resp.StatusCode, resp.Header, nil
}

// get executes a GET request to the given endpoint with the given query params
func (client *Client) get(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	req, err := client.newRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	query := req.URL.Query()
	for key, value := range params {
		query.Add(key, value)
	}
	req.URL.RawQuery = query.Encode()

	var statusCode int

	operation := func() ([]byte, error) {
		err := client.authenticate(ctx)
		if err != nil {
			return nil, client.authRetryError(ctx, err)
		}

		var bytes []byte
		var header http.Header
		bytes, statusCode, header, err = client.do(req)
		if err != nil && ctx.Err() != nil {
			return nil, backoff.Permanent(ctx.Err())
		}
		if errors.Is(err, middleware.ErrRateLimitTimeout) {
			return nil, backoff.Permanent(err)
		}

		if err == nil && isValidStatusCode(statusCode) {
			return bytes, nil
		}

		return nil, client.retryError(statusCode, header, err)
	}

	bytes, err := backoff.Retry(ctx, operation,
		backoff.WithBackOff(client.retryBackOff()),
		backoff.WithMaxTries(uint(max(client.maxAttempts, 1))),
		backoff.WithMaxElapsedTime(client.maxRetryDuration),
		backoff.WithNotify(func(err error, wait time.Duration) {
			if wait > 0 {
				log.Debugf("Cisco sd-wan api request to %s failed (%s), retrying in %s", endpoint, err, wait)
			}
		}),
	)
	if err == nil {
		return bytes, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var authErr *authError
	if errors.Is(err, backoff.ErrPermanent) || errors.As(err, &authErr) {
		// Authentication or rate limiter wait failed, surface the underlying error
		return nil, backoff.AsRetryError(err).LastErr
	}

	return nil, fmt.Errorf("%s http responded with %d code", endpoint, statusCode)
}

func (client *Client) retryBackOff() backoff.BackOff {
	if !client.backoffEnabled {
		return &backoff.ZeroBackOff{}
	}
	return newRetryBackOff()
}

// authRetryError builds the error returned when authentication fails. When backoff is enabled,
// transient failures of the authentication requests are retried like API requests, honoring
// Retry-After. Everything else, including invalid credentials, rate limiter and cancellation
// errors, stops retrying.
func (client *Client) authRetryError(ctx context.Context, err error) error {
	var authErr *authError
	if ctx.Err() == nil && client.backoffEnabled && errors.As(err, &authErr) && authErr.transient() {
		return client.retryError(authErr.statusCode, authErr.header, err)
	}
	return backoff.Permanent(err)
}

// retryError builds the error returned for a failed attempt. When backoff is enabled,
// transient failures wait for the backoff policy (or the server-provided Retry-After);
// every other failure, including 401 which triggers re-authentication, is retried immediately.
func (client *Client) retryError(statusCode int, header http.Header, err error) error {
	retryable := client.backoffEnabled && isRetryable(statusCode, err)
	if err == nil {
		err = fmt.Errorf("http responded with %d code", statusCode)
	}

	if !retryable {
		return backoff.RetryAfter(0, err)
	}

	if retryAfter := parseRetryAfter(header); retryAfter > 0 {
		return backoff.RetryAfter(min(retryAfter, maxRetryBackoff), err)
	}

	return err
}

func isRetryable(statusCode int, err error) bool {
	if err != nil {
		return true
	}
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

// parseRetryAfter parses the Retry-After header, which may be a number of seconds or
// an HTTP date. It returns 0 when the header is absent or invalid.
func parseRetryAfter(header http.Header) time.Duration {
	value := header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return date.Sub(timeNow())
	}
	return 0
}

// get wraps client.get with generic type content and unmarshalling (methods can't use generics)
func get[T Content](ctx context.Context, client *Client, endpoint string, params map[string]string) (*Response[T], error) {
	bytes, err := client.get(ctx, endpoint, params)
	if err != nil {
		return nil, err
	}

	var data Response[T]

	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func isValidStatusCode(code int) bool {
	return code >= 200 && code < 400
}

// getMoreEntries gets all results from paginated endpoints
func getMoreEntries[T Content](ctx context.Context, client *Client, endpoint string, pageInfo PageInfo) ([]T, error) {
	var responses []T
	currentPageInfo := pageInfo

	// Loop while API response indicates there is more entries
	for page := 0; currentPageInfo.MoreEntries || currentPageInfo.HasMoreData; page++ {
		// Error if max number of pages is reached
		if page >= client.maxPages {
			return nil, errors.New("max number of page reached, increase API count or max number of pages")
		}

		log.Tracef("Getting page %d from endpoint %s", page+1+1, endpoint)
		// Build pagination parameters for the to get next API page
		nextParams, err := getNextPaginationParams(currentPageInfo, client.maxCount)
		if err != nil {
			return nil, err
		}
		log.Tracef("Pagination params for page %d from endpoint %s : %v", page+1+1, endpoint, nextParams)

		// Call the endpoint with the new params
		data, err := get[T](ctx, client, endpoint, nextParams)
		if err != nil {
			return nil, err
		}

		responses = append(responses, data.Data...)
		currentPageInfo = data.PageInfo
	}

	return responses, nil
}

// getNextPaginationParams builds query params to get next page
func getNextPaginationParams(info PageInfo, count string) (map[string]string, error) {
	newParams := make(map[string]string)
	if info.MoreEntries {
		// For endpoints that uses index-based pagination
		newParams["count"] = count
		newParams["startId"] = info.EndID
		return newParams, nil
	} else if info.HasMoreData {
		// For endpoints that uses scroll-based pagination (ES like)
		newParams["scrollId"] = info.ScrollID
		return newParams, nil
	}
	return nil, errors.New("could not build next page params")
}

// getAllEntries gets all entries from paginated endpoints
func getAllEntries[T Content](ctx context.Context, client *Client, endpoint string, params map[string]string) (*Response[T], error) {
	data, err := get[T](ctx, client, endpoint, params)
	if err != nil {
		return nil, err
	}

	// If API response is paginated, get the rest
	entries, err := getMoreEntries[T](ctx, client, endpoint, data.PageInfo)
	if err != nil {
		return nil, err
	}

	data.Data = append(data.Data, entries...)

	return data, nil
}
