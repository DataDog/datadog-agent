// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// baseRetryBackoff is the initial wait before retrying a transient failure
	baseRetryBackoff = 1 * time.Second
	// maxRetryBackoff caps the wait between retries, including server-provided Retry-After values
	maxRetryBackoff = 30 * time.Second
)

// newRetryBackOff builds the exponential backoff policy used when backoff is enabled.
// Useful for mocking
var newRetryBackOff = func() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = baseRetryBackoff
	b.Multiplier = 2
	b.MaxInterval = maxRetryBackoff
	return b
}

// newRequest creates a new request for this client.
func (client *Client) newRequest(method, uri string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(client.ctx, method, client.endpoint+uri, body)
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

	var statusCode int

	operation := func() ([]byte, error) {
		err := client.authenticate()
		if err != nil {
			return nil, backoff.Permanent(err)
		}

		// Retrying would not help if the rate limiter wait failed or the check was cancelled
		err = client.waitForRateLimit()
		if err != nil {
			return nil, backoff.Permanent(err)
		}

		var bytes []byte
		var header http.Header
		bytes, statusCode, header, err = client.do(req)
		if client.ctx.Err() != nil {
			return nil, backoff.Permanent(client.ctx.Err())
		}

		if err == nil && isValidStatusCode(statusCode) {
			// Got a valid response, stop retrying
			return bytes, nil
		}

		return nil, client.retryError(statusCode, header, err)
	}

	// The client context interrupts backoff waits when the check is cancelled
	bytes, err := backoff.Retry(client.ctx, operation,
		backoff.WithBackOff(client.retryBackOff()),
		backoff.WithMaxTries(uint(max(client.maxAttempts, 1))),
		// Attempts are bounded by maxAttempts, each wait by maxRetryBackoff and all of them by maxRetryDuration
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
	if client.ctx.Err() != nil {
		return nil, client.ctx.Err()
	}
	if errors.Is(err, backoff.ErrPermanent) {
		// Authentication or rate limiter wait failed, surface the underlying error
		return nil, backoff.AsRetryError(err).LastErr
	}

	return nil, fmt.Errorf("%s http responded with %d code", endpoint, statusCode)
}

// retryBackOff returns the policy used between attempts. Without backoff enabled,
// failed requests are retried immediately.
func (client *Client) retryBackOff() backoff.BackOff {
	if !client.backoffEnabled {
		return &backoff.ZeroBackOff{}
	}
	return newRetryBackOff()
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

// isRetryable reports whether a failed request is worth waiting on and retrying:
// network errors, rate-limiting (429) and transient server errors (5xx). Auth
// failures (401) are excluded as they trigger immediate re-authentication.
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

	return &data, nil
}

func isValidStatusCode(code int) bool {
	return code >= 200 && code < 400
}

// getMoreEntries gets all results from paginated endpoints
func getMoreEntries[T Content](client *Client, endpoint string, pageInfo PageInfo) ([]T, error) {
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
		data, err := get[T](client, endpoint, nextParams)
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
func getAllEntries[T Content](client *Client, endpoint string, params map[string]string) (*Response[T], error) {
	data, err := get[T](client, endpoint, params)
	if err != nil {
		return nil, err
	}

	// If API response is paginated, get the rest
	entries, err := getMoreEntries[T](client, endpoint, data.PageInfo)
	if err != nil {
		return nil, err
	}

	data.Data = append(data.Data, entries...)

	return data, nil
}
