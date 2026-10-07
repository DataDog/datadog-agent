// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client/fixtures"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client/middleware"
)

func TestNewRequest(t *testing.T) {
	tests := []struct {
		name          string
		host          string
		method        string
		uri           string
		expectedError string
	}{
		{
			name:   "get request",
			host:   "testserver",
			method: "GET",
			uri:    "/test-endpoint",
		},
		{
			name:   "post request",
			host:   "testserver2",
			method: "POST",
			uri:    "/post-endpoint",
		},
		{
			name:          "invalid method",
			host:          "testserver2",
			method:        "HELLO?",
			uri:           "/endpoint",
			expectedError: "invalid method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.host, "testuser", "testpassword", false)
			require.NoError(t, err)

			req, err := client.newRequest(tt.method, tt.uri, nil)

			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.method, req.Method)
			require.Equal(t, tt.host, req.URL.Host)
			require.Equal(t, tt.uri, req.URL.RequestURI())
		})
	}
}

func TestDoRequest(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, _ int32) {
		token := r.Header.Get("X-XSRF-TOKEN")
		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set token
	client.token = "testtoken"

	req, err := http.NewRequest("GET", "http://"+serverURL(server)+"/test", nil)
	require.NoError(t, err)

	body, statusCode, _, err := client.do(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, statusCode)
	require.Equal(t, []byte(""), body)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestDoRequestBadRequest(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, _ int32) {
		token := r.Header.Get("X-XSRF-TOKEN")
		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("an error occurred"))
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set token
	client.token = "testtoken"

	req, err := http.NewRequest("GET", "http://"+serverURL(server)+"/test", nil)
	require.NoError(t, err)

	body, statusCode, _, err := client.do(req)

	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, statusCode)
	require.Equal(t, []byte("an error occurred"), body)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestDoRequestError(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, _ int32) {
		token := r.Header.Get("X-XSRF-TOKEN")
		// Ensure token is correctly passed as header
		require.Equal(t, "testtoken", token)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set token
	client.token = "testtoken"

	// Create request with invalid URL
	req, err := http.NewRequest("GET", "", nil)
	require.NoError(t, err)

	body, statusCode, _, err := client.do(req)

	require.ErrorContains(t, err, "unsupported protocol scheme")
	require.Equal(t, 0, statusCode)
	require.Equal(t, []byte(nil), body)
	require.Equal(t, 0, handler.numberOfCalls())
}

func TestGetRequest(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, r *http.Request, _ int32) {
		query := r.URL.Query()
		test := query.Get("test")
		test2 := query.Get("test2")

		// URL params are correctly set
		require.Equal(t, "param", test)
		require.Equal(t, "param2", test2)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte{})
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	params := map[string]string{
		"test":  "param",
		"test2": "param2",
	}

	resp, err := client.get("/test", params)
	require.NoError(t, err)
	require.Equal(t, []byte{}, resp)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetRequestRetries(t *testing.T) {
	mux := setupCommonServerMux()
	handler := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("an error occurred"))
	})

	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set max retries to 10 for testing
	client.maxAttempts = 10

	resp, err := client.get("/test", nil)
	require.ErrorContains(t, err, "http responded with 400 code")
	require.Equal(t, []byte(nil), resp)
	require.Equal(t, 10, handler.numberOfCalls())
}

// countingBackOff never waits and records how many times the retry loop asked for a backoff
type countingBackOff struct {
	calls int
}

func (b *countingBackOff) Reset() {}

func (b *countingBackOff) NextBackOff() time.Duration {
	b.calls++
	return 0
}

func rateLimitedHandler(failures int32) handler {
	return newHandler(func(w http.ResponseWriter, _ *http.Request, calls int32) {
		w.Header().Set("Content-Type", "application/json")
		// Rate-limit the first attempts, then succeed
		if calls <= failures {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

func TestGetRequestBacksOffExponentially(t *testing.T) {
	policy := &countingBackOff{}
	originalBackOff := newRetryBackOff
	newRetryBackOff = func() backoff.BackOff { return policy }
	defer func() { newRetryBackOff = originalBackOff }()

	mux := setupCommonServerMux()
	handler := rateLimitedHandler(2) // mock "server" returns 2 failures, then success
	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	client.maxAttempts = 5
	client.backoffEnabled = true

	resp, err := client.get("/test", nil)
	require.NoError(t, err)
	require.Equal(t, []byte("ok"), resp)
	require.Equal(t, 3, handler.numberOfCalls()) // 3 total calls made
	require.Equal(t, 2, policy.calls)            // 2 backoff calls were made
}

func TestGetRequestBackoffDisabledByDefault(t *testing.T) {
	policy := &countingBackOff{}
	originalBackOff := newRetryBackOff
	newRetryBackOff = func() backoff.BackOff { return policy }
	defer func() { newRetryBackOff = originalBackOff }()

	mux := setupCommonServerMux()
	handler := rateLimitedHandler(2) // mock "server" returns 2 failures, then success
	mux.HandleFunc("/test", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	client.maxAttempts = 5

	resp, err := client.get("/test", nil)
	require.NoError(t, err)
	require.Equal(t, []byte("ok"), resp)
	require.Equal(t, 3, handler.numberOfCalls()) // 3 total calls made
	require.Equal(t, 0, policy.calls)            // no exponential backoff was used
}

// loginFailingMux serves the API with a login endpoint that answers with failure for its first calls
func loginFailingMux(failures int32, failure func(w http.ResponseWriter)) (*http.ServeMux, handler, handler) {
	login := newHandler(func(w http.ResponseWriter, _ *http.Request, calls int32) {
		if calls <= failures {
			failure(w)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	endpoint := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/j_security_check", login.Func)
	mux.HandleFunc("/dataservice/client/token", tokenHandler)
	mux.HandleFunc("/test", endpoint.Func)
	return mux, login, endpoint
}

func TestGetRequestAuthFailureBackoff(t *testing.T) {
	status := func(code int) func(w http.ResponseWriter) {
		return func(w http.ResponseWriter) { w.WriteHeader(code) }
	}
	invalidCredentials := func(w http.ResponseWriter) {
		// Cisco answers invalid credentials with the HTML login page
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>login</html>"))
	}

	tests := []struct {
		name               string
		backoffEnabled     bool
		failure            func(w http.ResponseWriter)
		expectedError      string
		expectedLoginCalls int
		expectedBackoffs   int
	}{
		{name: "rate limited login backs off", backoffEnabled: true, failure: status(http.StatusTooManyRequests), expectedLoginCalls: 3, expectedBackoffs: 2},
		{name: "server error on login backs off", backoffEnabled: true, failure: status(http.StatusServiceUnavailable), expectedLoginCalls: 3, expectedBackoffs: 2},
		{name: "invalid credentials stop retrying", backoffEnabled: true, failure: invalidCredentials, expectedError: "invalid credentials", expectedLoginCalls: 1},
		{name: "forbidden login stops retrying", backoffEnabled: true, failure: status(http.StatusForbidden), expectedError: "authentication failed, status code: 403", expectedLoginCalls: 1},
		{name: "backoff disabled stops retrying", backoffEnabled: false, failure: status(http.StatusServiceUnavailable), expectedError: "authentication failed, status code: 503", expectedLoginCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &countingBackOff{}
			originalBackOff := newRetryBackOff
			newRetryBackOff = func() backoff.BackOff { return policy }
			defer func() { newRetryBackOff = originalBackOff }()

			mux, login, endpoint := loginFailingMux(2, tt.failure)
			server := httptest.NewServer(mux)
			defer server.Close()

			client, err := testClient(server, WithMaxAttempts(5), WithBackoff(tt.backoffEnabled))
			require.NoError(t, err)

			resp, err := client.get("/test", nil)
			if tt.expectedError == "" {
				require.NoError(t, err)
				require.Equal(t, []byte("ok"), resp)
				require.Equal(t, 1, endpoint.numberOfCalls())
			} else {
				require.ErrorContains(t, err, tt.expectedError)
				require.Equal(t, 0, endpoint.numberOfCalls())
			}
			require.Equal(t, tt.expectedLoginCalls, login.numberOfCalls())
			require.Equal(t, tt.expectedBackoffs, policy.calls)
		})
	}
}

func TestGetRequestAuthFailureExhaustsAttempts(t *testing.T) {
	policy := &countingBackOff{}
	originalBackOff := newRetryBackOff
	newRetryBackOff = func() backoff.BackOff { return policy }
	defer func() { newRetryBackOff = originalBackOff }()

	mux, login, _ := loginFailingMux(10, func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) })
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server, WithMaxAttempts(3), WithBackoff(true))
	require.NoError(t, err)

	_, err = client.get("/test", nil)
	// The authentication failure is reported, not a misleading API status code
	require.EqualError(t, err, "authentication failed, status code: 503")
	require.Equal(t, 3, login.numberOfCalls())
	require.Equal(t, 2, policy.calls)
}

func TestGetRequestAuthFailureHonorsRetryAfter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux, login, _ := loginFailingMux(1, func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
		})

		client, err := NewClient("sdwan.test", "testuser", "testpass", true, WithBackoff(true))
		require.NoError(t, err)
		transport := &recordingTransport{mux: mux, start: time.Now()}
		client.httpClient.Transport = transport

		_, err = client.get("/test", nil)
		require.NoError(t, err)
		require.Equal(t, 2, login.numberOfCalls())
		// Login retried after the server-provided 5s, then token and API requests followed
		require.Equal(t, []time.Duration{0, 5 * time.Second, 5 * time.Second, 5 * time.Second}, transport.sent)
	})
}

// recordingTransport serves requests from a mux without opening sockets, so it can be used
// inside a synctest bubble, and records the virtual time at which each request is sent
type recordingTransport struct {
	mux   *http.ServeMux
	start time.Time
	sent  []time.Duration
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.sent = append(rt.sent, time.Since(rt.start))
	recorder := httptest.NewRecorder()
	rt.mux.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

func rateLimitedTestClient(t *testing.T, options ...ClientOptions) (*Client, *recordingTransport, handler) {
	mux, handler := setupCommonServerMuxWithFixture("/test", "")
	transport := &recordingTransport{mux: mux, start: time.Now()}
	// The base transport must be set as an option to be wrapped by the rate limiter
	client, err := NewClient("sdwan.test", "testuser", "testpass", true, append(options, WithTransport(transport))...)
	require.NoError(t, err)
	return client, transport, handler
}

func TestGetRequestRateLimited(t *testing.T) {
	tests := []struct {
		name         string
		burst        int
		expectedSent []time.Duration
	}{
		{
			name:  "burst of 1 paces every request",
			burst: 1,
			// 2 login requests, then 3 GET requests
			expectedSent: []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond, 400 * time.Millisecond},
		},
		{
			name:         "burst of 3 sends the first 3 requests at once",
			burst:        3,
			expectedSent: []time.Duration{0, 0, 0, 100 * time.Millisecond, 200 * time.Millisecond},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client, transport, handler := rateLimitedTestClient(t, WithRateLimit(10, tt.burst, time.Second))

				for i := 0; i < 3; i++ {
					_, err := client.get("/test", nil)
					require.NoError(t, err)
				}

				require.Equal(t, tt.expectedSent, transport.sent)
				require.Equal(t, 3, handler.numberOfCalls())
			})
		})
	}
}

func TestGetRequestRateLimitMaxWait(t *testing.T) {
	tests := []struct {
		name         string
		burst        int
		backoff      bool
		expectedSent []time.Duration
	}{
		{
			name:         "login request refused",
			burst:        1,
			expectedSent: []time.Duration{0},
		},
		{
			name:         "login request refused with backoff",
			burst:        1,
			backoff:      true,
			expectedSent: []time.Duration{0},
		},
		{
			name:         "API request refused with backoff",
			burst:        2,
			backoff:      true,
			expectedSent: []time.Duration{0, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Once the burst is used, the next request would wait ~1000s for a token
				client, transport, handler := rateLimitedTestClient(t, WithBackoff(tt.backoff), WithRateLimit(0.001, tt.burst, 100*time.Millisecond))

				_, err := client.get("/test", nil)
				require.ErrorIs(t, err, middleware.ErrRateLimitTimeout)
				// The limiter fails immediately instead of waiting for a token it cannot get in time,
				// and the failure is not retried
				require.Equal(t, tt.expectedSent, transport.sent)
				require.Zero(t, time.Since(transport.start))
				require.Equal(t, 0, handler.numberOfCalls())
			})
		})
	}
}

func TestGetRequestRateLimitWaitExcludedFromTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Requests are paced 20s apart, longer than the HTTP timeout
		client, transport, handler := rateLimitedTestClient(t, WithRateLimit(0.05, 1, time.Minute))

		_, err := client.get("/test", nil)
		require.NoError(t, err)
		require.Equal(t, []time.Duration{0, 20 * time.Second, 40 * time.Second}, transport.sent)
		require.Equal(t, 1, handler.numberOfCalls())
	})
}

func TestGetRequestRateLimitCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client, transport, handler := rateLimitedTestClient(t, WithContext(ctx), WithRateLimit(0.01, 1, time.Hour))

		time.AfterFunc(100*time.Millisecond, cancel)

		_, err := client.get("/test", nil)
		require.ErrorIs(t, err, context.Canceled)
		// The second login request is interrupted by the cancellation, not by a token becoming available
		require.Equal(t, []time.Duration{0}, transport.sent)
		require.Equal(t, 100*time.Millisecond, time.Since(transport.start))
		require.Equal(t, 0, handler.numberOfCalls())
	})
}

func TestGetRequestBackoffCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		mux := setupCommonServerMux()
		handler := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		mux.HandleFunc("/test", handler.Func)

		client, err := NewClient("sdwan.test", "testuser", "testpass", true, WithContext(ctx), WithBackoff(true), WithMaxAttempts(3))
		require.NoError(t, err)
		transport := &recordingTransport{mux: mux, start: time.Now()}
		client.httpClient.Transport = transport

		time.AfterFunc(100*time.Millisecond, cancel)

		_, err = client.get("/test", nil)
		require.ErrorIs(t, err, context.Canceled)
		// The 30s Retry-After wait is interrupted by the cancellation
		require.Equal(t, 100*time.Millisecond, time.Since(transport.start))
		require.Equal(t, 1, handler.numberOfCalls())
	})
}

func TestGetRequestMaxRetryDuration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mux := setupCommonServerMux()
		handler := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
		})
		mux.HandleFunc("/test", handler.Func)

		client, err := NewClient("sdwan.test", "testuser", "testpass", true, WithBackoff(true), WithMaxAttempts(10), WithMaxRetryDuration(time.Minute))
		require.NoError(t, err)
		transport := &recordingTransport{mux: mux, start: time.Now()}
		client.httpClient.Transport = transport

		_, err = client.get("/test", nil)
		require.ErrorContains(t, err, "http responded with 503 code")
		// Attempts at 0s, 30s and 60s, then a 4th wait would end after the 1m budget
		require.Equal(t, 3, handler.numberOfCalls())
		require.Equal(t, time.Minute, time.Since(transport.start))
	})
}

func TestGetRequestUnmarshalling(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture("/test", fixtures.FakePayload(fixtures.GetDevices))

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	resp, err := get[Device](client, "/test", nil)
	require.NoError(t, err)
	require.Equal(t, "10.10.1.1", resp.Data[0].DeviceID)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetRequestUnmarshallingError(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture("/test", fixtures.FakePayload(`
[
	{
		"lastupdated": "1.0"
	}
]
`))

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	resp, err := get[Device](client, "/test", nil)
	var typeErr *json.UnmarshalTypeError
	require.ErrorAs(t, err, &typeErr)
	require.Equal(t, "string", typeErr.Value)
	require.Contains(t, typeErr.Field, "lastupdated")

	var empty *Response[Device]
	require.Equal(t, empty, resp)
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetMoreEntriesMaxPages(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, _ *http.Request, _ int32) {
		// Always respond saying that more entries are available
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			{
  				"pageInfo": {
       	 			"startId": "31",
        			"endId": "35",
        			"moreEntries": true,
        			"count": 4
    			}
			}
		`))
	})

	mux.Handle("/dataservice/device", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	// Set max pages to 20 for testing
	client.maxPages = 20

	pageInfo := PageInfo{
		StartID:     "1",
		EndID:       "15",
		MoreEntries: true,
	}

	_, err = getMoreEntries[Device](client, "/dataservice/device", pageInfo)
	require.ErrorContains(t, err, "max number of page reached")

	// Ensure endpoint has been called 20 times
	require.Equal(t, 20, handler.numberOfCalls())
}

func TestGetMoreEntriesIndexPagination(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		startID := r.URL.Query().Get("startId")
		if calls == 1 {
			// First call, expect startId to be 15

			require.Equal(t, "15", startID, "startId should be correct")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
				{
  					"pageInfo": {
        				"startId": "15",
						"endId": "30",
        				"moreEntries": true,
        				"count": 14
					}
				}
			`))
			return
		}

		// Second call, expect startId to be 31
		require.Equal(t, "30", startID, "startId should be correct")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			{
  				"pageInfo": {
       	 			"startId": "30",
        			"endId": "35",
        			"moreEntries": false,
        			"count": 4
    			}
			}
		`))
	})

	mux.Handle("/dataservice/device", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	pageInfo := PageInfo{
		StartID:     "1",
		EndID:       "15",
		MoreEntries: true,
	}

	_, err = getMoreEntries[Device](client, "/dataservice/device", pageInfo)
	require.NoError(t, err)

	// Ensure endpoint has been called 2 times
	require.Equal(t, 2, handler.numberOfCalls())
}

func TestGetMoreEntriesScrollPagination(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		scrollID := r.URL.Query().Get("scrollId")
		startDate := r.URL.Query().Get("startDate")
		endDate := r.URL.Query().Get("endDate")
		timeZone := r.URL.Query().Get("timeZone")
		count := r.URL.Query().Get("count")

		if calls == 1 {
			// First call, expect scrollId to be "test", and no other params

			require.Equal(t, "test", scrollID, "scrollId should be correct")
			require.Empty(t, startDate)
			require.Empty(t, endDate)
			require.Empty(t, timeZone)
			require.Empty(t, count)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`
				{
  					"pageInfo": {
        				"scrollId": "test2",
        				"hasMoreData": true
					}
				}
			`))
			return
		}

		// Second call, expect scrollId to be "test2"
		require.Equal(t, "test2", scrollID, "scrollId should be correct")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
			{
  				"pageInfo": {
       	 			"scrollId": "test2",
					"hasMoreData": false
    			}
			}
		`))
	})

	mux.Handle("/dataservice/device", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	pageInfo := PageInfo{
		ScrollID:    "test",
		HasMoreData: true,
	}

	_, err = getMoreEntries[Device](client, "/dataservice/device", pageInfo)
	require.NoError(t, err)

	// Ensure endpoint has been called 2 times
	require.Equal(t, 2, handler.numberOfCalls())
}

func TestGetNextPaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		pageInfo       PageInfo
		count          string
		expectedParams map[string]string
		expectedError  string
	}{
		{
			name:  "index based pagination",
			count: "1000",
			pageInfo: PageInfo{
				StartID:     "0",
				EndID:       "10",
				MoreEntries: true,
				Count:       10,
			},
			expectedParams: map[string]string{"count": "1000", "startId": "10"},
		},
		{
			name:  "scroll based pagination",
			count: "1000",
			pageInfo: PageInfo{
				HasMoreData: true,
				ScrollID:    "test",
				Count:       10,
			},
			expectedParams: map[string]string{"scrollId": "test"},
		},
		{
			name:           "invalid page info",
			count:          "1000",
			pageInfo:       PageInfo{},
			expectedError:  "could not build next page params",
			expectedParams: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextParams, err := getNextPaginationParams(tt.pageInfo, tt.count)
			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
			}
			require.Equal(t, tt.expectedParams, nextParams)
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		err        error
		retryable  bool
	}{
		{name: "network error", statusCode: 0, err: errors.New("connection reset"), retryable: true},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, retryable: true},
		{name: "server error", statusCode: http.StatusServiceUnavailable, retryable: true},
		{name: "auth failure", statusCode: http.StatusUnauthorized, retryable: false},
		{name: "bad request", statusCode: http.StatusBadRequest, retryable: false},
		{name: "success", statusCode: http.StatusOK, retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.retryable, isRetryable(tt.statusCode, tt.err))
		})
	}
}

func TestRetryBackOffCapped(t *testing.T) {
	policy := newRetryBackOff()
	policy.Reset()

	// Enough attempts to saturate the interval: 1s doubling reaches the cap after a few
	var saturated []time.Duration
	for i := 0; i < 1000; i++ {
		wait := policy.NextBackOff()
		require.Positive(t, wait)
		require.LessOrEqual(t, wait, maxRetryBackoff)
		if i >= 10 {
			saturated = append(saturated, wait)
		}
	}
	// Jitter is preserved once saturated instead of collapsing onto the cap
	require.Less(t, slices.Min(saturated), maxRetryBackoff/2)
	require.NotEqual(t, slices.Min(saturated), slices.Max(saturated))
}

func TestCappedBackOff(t *testing.T) {
	capped := &cappedBackOff{BackOff: backoff.NewConstantBackOff(45 * time.Second), max: maxRetryBackoff}
	require.Equal(t, maxRetryBackoff, capped.NextBackOff())

	capped = &cappedBackOff{BackOff: backoff.NewConstantBackOff(time.Second), max: maxRetryBackoff}
	require.Equal(t, time.Second, capped.NextBackOff())

	capped = &cappedBackOff{BackOff: &backoff.StopBackOff{}, max: maxRetryBackoff}
	require.Equal(t, backoff.Stop, capped.NextBackOff())
}

func TestRetryError(t *testing.T) {
	retryAfter := func(value string) http.Header {
		return http.Header{"Retry-After": []string{value}}
	}

	tests := []struct {
		name           string
		backoffEnabled bool
		statusCode     int
		header         http.Header
		err            error
		// expectedWait is the forced wait, or -1 when the backoff policy decides
		expectedWait time.Duration
	}{
		{name: "disabled retries immediately", backoffEnabled: false, statusCode: http.StatusTooManyRequests, header: retryAfter("2"), expectedWait: 0},
		{name: "honors Retry-After", backoffEnabled: true, statusCode: http.StatusTooManyRequests, header: retryAfter("2"), expectedWait: 2 * time.Second},
		{name: "caps Retry-After", backoffEnabled: true, statusCode: http.StatusTooManyRequests, header: retryAfter("3600"), expectedWait: maxRetryBackoff},
		{name: "invalid Retry-After uses policy", backoffEnabled: true, statusCode: http.StatusTooManyRequests, header: retryAfter("soon"), expectedWait: -1},
		{name: "server error uses policy", backoffEnabled: true, statusCode: http.StatusServiceUnavailable, header: http.Header{}, expectedWait: -1},
		{name: "network error uses policy", backoffEnabled: true, err: errors.New("connection reset"), expectedWait: -1},
		{name: "auth failure retries immediately", backoffEnabled: true, statusCode: http.StatusUnauthorized, expectedWait: 0},
		{name: "bad request retries immediately", backoffEnabled: true, statusCode: http.StatusBadRequest, expectedWait: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{backoffEnabled: tt.backoffEnabled}
			err := client.retryError(tt.statusCode, tt.header, tt.err)
			require.Error(t, err)

			var forced *backoff.RetryAfterError
			if tt.expectedWait < 0 {
				require.False(t, errors.As(err, &forced))
				return
			}
			require.True(t, errors.As(err, &forced))
			require.Equal(t, tt.expectedWait, forced.Duration)
		})
	}
}

func TestIsValidStatusCode(t *testing.T) {
	tests := []struct {
		code    int
		isValid bool
	}{
		{
			code:    200,
			isValid: true,
		},
		{
			code:    201,
			isValid: true,
		},
		{
			code:    299,
			isValid: true,
		},
		{
			code:    399,
			isValid: true,
		},
		{
			code:    400,
			isValid: false,
		},
		{
			code:    401,
			isValid: false,
		},
		{
			code:    1839849230,
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run("is valid status code", func(t *testing.T) {
			require.Equal(t, tt.isValid, isValidStatusCode(tt.code))
		})
	}
}
