package appsec

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
)

func TestIntakeReverseProxy(t *testing.T) {
	defer func(old coreconfig.Config) { coreconfig.Datadog = old }(coreconfig.Datadog)

	t.Run("appsec enabled by default", func(t *testing.T) {
		config := coreconfig.Mock()
		coreconfig.Datadog = config

		proxy, err := NewIntakeReverseProxy(http.DefaultTransport)
		require.NoError(t, err)
		require.NotNil(t, proxy)
	})

	t.Run("appsec disabled", func(t *testing.T) {
		config := coreconfig.Mock()
		coreconfig.Datadog = config

		config.Set("appsec_config.enabled", false)
		proxy, err := NewIntakeReverseProxy(http.DefaultTransport)
		require.NoError(t, err)
		require.NotNil(t, proxy)

		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("configuration error", func(t *testing.T) {
		config := coreconfig.Mock()
		coreconfig.Datadog = config

		config.Set("site", "not a site")
		proxy, err := NewIntakeReverseProxy(http.DefaultTransport)
		require.Error(t, err)
		require.NotNil(t, proxy)

		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("proxy handler", func(t *testing.T) {
		// Helper value and functions

		const (
			expectedAPIKey         = "an api key"
			expectedServerEndpoint = "/server/endpoint"
			expectedMaxPayloadSize = 5 * 1024 * 1024
		)

		requireProxyHeaders := func(t *testing.T, req *http.Request) {
			require.Contains(t, "trace-agent", req.Header.Get("Via"))
			require.Equal(t, expectedAPIKey, req.Header.Get("Dd-Api-Key"))
		}
		requireRequest := func(t *testing.T, req *http.Request, expectedMethod, expectedEndpoint string, expectedBody []byte) {
			require.Equal(t, expectedMethod, req.Method)
			require.Equal(t, expectedServerEndpoint+expectedEndpoint, req.URL.String())
			if expectedBody != nil {
				body, err := ioutil.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, expectedBody, body)
			}
		}

		randomBody := make([]byte, expectedMaxPayloadSize-1)
		n, err := rand.Read(randomBody)
		require.NoError(t, err)
		require.Equal(t, len(randomBody), n)

		for _, tc := range []struct {
			name string
			// The function creating the fake server request to be handled by the proxy
			prepareServerRequest func(*testing.T) *http.Request
			// The function handling the server requests the proxy sends its requests to
			targetHandler func(*testing.T, http.ResponseWriter, *http.Request)
			// The function testing the recorded proxy response
			testResponse func(*testing.T, *httptest.ResponseRecorder)
		}{
			{
				name: "proxy headers",
				prepareServerRequest: func(t *testing.T) *http.Request {
					return httptest.NewRequest("POST", "/my/endpoint/1", nil)
				},
				targetHandler: func(t *testing.T, w http.ResponseWriter, req *http.Request) {
					requireProxyHeaders(t, req)
					requireRequest(t, req, "POST", "/my/endpoint/1", nil)
					w.WriteHeader(201)
				},
				testResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
					require.Equal(t, 201, rec.Code)
				},
			},
			{
				name: "original request headers",
				prepareServerRequest: func(t *testing.T) *http.Request {
					req := httptest.NewRequest("GET", "/my/endpoint/2", nil)
					req.Header.Add("My-Header-1", "my value 1")
					req.Header.Add("My-Header-2", "my value 2")
					req.Header.Add("My-Header-3", "my value 3")
					req.Header.Add("My-Header-4", "my value 4")
					return req
				},
				targetHandler: func(t *testing.T, w http.ResponseWriter, req *http.Request) {
					requireProxyHeaders(t, req)
					requireRequest(t, req, "GET", "/my/endpoint/2", nil)
					require.Equal(t, "my value 1", req.Header.Get("My-Header-1"))
					require.Equal(t, "my value 2", req.Header.Get("My-Header-2"))
					require.Equal(t, "my value 3", req.Header.Get("My-Header-3"))
					require.Equal(t, "my value 4", req.Header.Get("My-Header-4"))
					w.WriteHeader(202)
				},
				testResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
					require.Equal(t, 202, rec.Code)
				},
			},
			{
				name: "original request body",
				prepareServerRequest: func(t *testing.T) *http.Request {
					return httptest.NewRequest("PUT", "/my/endpoint/3", bytes.NewReader(randomBody))
				},
				targetHandler: func(t *testing.T, w http.ResponseWriter, req *http.Request) {
					requireProxyHeaders(t, req)
					requireRequest(t, req, "PUT", "/my/endpoint/3", randomBody)
					w.WriteHeader(203)
				},
				testResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
					require.Equal(t, 203, rec.Code)
				},
			},
			{
				name: "request body too large",
				prepareServerRequest: func(t *testing.T) *http.Request {
					body := append(randomBody, randomBody...)
					return httptest.NewRequest("PUT", "/my/endpoint/3", bytes.NewReader(body))
				},
				targetHandler: func(t *testing.T, w http.ResponseWriter, req *http.Request) {
					requireProxyHeaders(t, req)
					requireRequest(t, req, "PUT", "/my/endpoint/3", nil)
					_, err := ioutil.ReadAll(req.Body)
					// a server-side error occurs because the proxy cancels the request
					// when the max payload size being reach
					require.Error(t, err)
				},
				testResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
					require.Equal(t, http.StatusInternalServerError, rec.Code)
				},
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					tc.targetHandler(t, w, req)
				}))
				defer srv.Close()

				url, err := url.Parse(srv.URL + expectedServerEndpoint)
				require.NoError(t, err)
				proxy := newIntakeReverseProxy(url, expectedAPIKey, expectedMaxPayloadSize, http.DefaultTransport)

				req := tc.prepareServerRequest(t)
				rec := httptest.NewRecorder()

				proxy.ServeHTTP(rec, req)
				tc.testResponse(t, rec)
			})
		}
	})
}

func TestRoundTripper(t *testing.T) {
	t.Run("tags", func(t *testing.T) {
		for _, tc := range []struct {
			name         string
			request      http.Request
			expectedTags []string
		}{
			{
				name: "path only",
				request: http.Request{
					URL: &url.URL{
						Path: "/some/endpoint",
					},
				},
				expectedTags: []string{"path:/some/endpoint"},
			},
			{
				name: "path and content_type",
				request: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/json"},
					},
					URL: &url.URL{
						Path: "/some/endpoint",
					},
				},
				expectedTags: []string{"path:/some/endpoint", "content_type:application/json"},
			},
			{
				name: "path and payload_size",
				request: http.Request{
					URL: &url.URL{
						Path: "/some/endpoint",
					},
					Body: &apiutil.LimitedReader{Count: 1073741824},
				},
				expectedTags: []string{"path:/some/endpoint"},
			},
			{
				name: "path, content_type and payload_size",
				request: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/json"},
					},
					URL: &url.URL{
						Path: "/some/endpoint",
					},
					Body: &apiutil.LimitedReader{Count: 1025},
				},
				expectedTags: []string{"path:/some/endpoint", "content_type:application/json"},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, tc.expectedTags, metricsTags(&tc.request))
			})
		}
	})

	t.Run("metrics", func(t *testing.T) {
		randBodyBuf := make([]byte, 8192)
		_, err := rand.Read(randBodyBuf)
		require.NoError(t, err)

		for _, tc := range []struct {
			name               string
			req                http.Request
			maxPayloadSize     int64
			expectedError      bool
			roundTripperError  error
			testRequestMetrics func(*testing.T, *testutil.TestStatsClient)
		}{
			{
				name: "no body",
				req: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/json"},
					},
					URL: &url.URL{
						Path: "/some/endpoint",
					},
					Body: nil,
				},
				testRequestMetrics: func(t *testing.T, stats *testutil.TestStatsClient) {
					expectedTags := []string{
						"content_type:application/json",
						"path:/some/endpoint",
					}

					calls := stats.HistogramCalls
					require.Len(t, calls, 0)

					calls = stats.TimingCalls
					require.Len(t, calls, 1)
					require.Equal(t, appSecRequestDurationMetricsID, calls[0].Name)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)
					require.Equal(t, float64(1), calls[0].Rate)
					// Not testing the time duration value as it can be 0 on Windows due to its time resolution

					calls = stats.CountCalls
					require.Len(t, calls, 1)
					require.Equal(t, appSecRequestCountMetricsID, calls[0].Name)
					require.Equal(t, float64(1), calls[0].Value)
					require.Equal(t, float64(1), calls[0].Rate)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)
				},
			},
			{
				name: "body without reaching the size limit",
				req: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/msgpack"},
					},
					URL: &url.URL{
						Path: "/some/endpoint/2",
					},
					Body: ioutil.NopCloser(bytes.NewReader(randBodyBuf)),
				},
				maxPayloadSize: 10000,
				testRequestMetrics: func(t *testing.T, stats *testutil.TestStatsClient) {
					expectedTags := []string{
						"content_type:application/msgpack",
						"path:/some/endpoint/2",
					}

					calls := stats.HistogramCalls
					require.Len(t, calls, 0)

					calls = stats.TimingCalls
					require.Len(t, calls, 1)
					require.Equal(t, appSecRequestDurationMetricsID, calls[0].Name)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)
					require.Equal(t, float64(1), calls[0].Rate)
					// Not testing the time duration value as it can be 0 on Windows due to its time resolution

					calls = stats.CountCalls
					require.Len(t, calls, 1)
					require.Equal(t, appSecRequestCountMetricsID, calls[0].Name)
					require.Equal(t, float64(1), calls[0].Value)
					require.Equal(t, float64(1), calls[0].Rate)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)
				},
			},
			{
				name: "body reaching the size limit",
				req: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/cbor"},
					},
					URL: &url.URL{
						Path: "/some/endpoint/3",
					},
					Body: ioutil.NopCloser(bytes.NewReader(randBodyBuf)),
				},
				maxPayloadSize: 1000,
				expectedError:  true,
				testRequestMetrics: func(t *testing.T, stats *testutil.TestStatsClient) {
					expectedTags := []string{
						"content_type:application/cbor",
						"path:/some/endpoint/3",
					}

					calls := stats.HistogramCalls
					require.Len(t, calls, 0)

					calls = stats.TimingCalls
					require.Len(t, calls, 1)
					require.Equal(t, appSecRequestDurationMetricsID, calls[0].Name)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)
					require.Equal(t, float64(1), calls[0].Rate)
					// Not testing the time duration value as it can be 0 on Windows due to its time resolution

					calls = stats.CountCalls
					require.Len(t, calls, 2)
					require.Equal(t, appSecRequestCountMetricsID, calls[0].Name)
					require.Equal(t, float64(1), calls[0].Value)
					require.Equal(t, float64(1), calls[0].Rate)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)

					require.Equal(t, appSecRequestErrorMetricsID, calls[1].Name)
					require.Equal(t, float64(1), calls[1].Value)
					require.Equal(t, float64(1), calls[1].Rate)
					require.ElementsMatch(t, append(expectedTags, "error:ErrLimitedReaderLimitReached"), calls[1].Tags)
				},
			},
			{
				name: "round tripper error",
				req: http.Request{
					Header: map[string][]string{
						"Content-Type": {"application/protobuf"},
					},
					URL: &url.URL{
						Path: "/some/endpoint/4",
					},
					Body: ioutil.NopCloser(bytes.NewReader(randBodyBuf)),
				},
				expectedError:     true,
				roundTripperError: errors.New("my error"),
				testRequestMetrics: func(t *testing.T, stats *testutil.TestStatsClient) {
					expectedTags := []string{
						"content_type:application/protobuf",
						"path:/some/endpoint/4",
					}

					calls := stats.TimingCalls
					require.Len(t, calls, 1)
					require.Equal(t, appSecRequestDurationMetricsID, calls[0].Name)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)
					require.Equal(t, float64(1), calls[0].Rate)
					// Not testing the time duration value as it can be 0 on Windows due to its time resolution

					calls = stats.CountCalls
					require.Len(t, calls, 2)
					require.Equal(t, appSecRequestCountMetricsID, calls[0].Name)
					require.Equal(t, float64(1), calls[0].Value)
					require.Equal(t, float64(1), calls[0].Rate)
					require.ElementsMatch(t, expectedTags, calls[0].Tags)

					require.Equal(t, appSecRequestErrorMetricsID, calls[1].Name)
					require.Equal(t, float64(1), calls[1].Value)
					require.Equal(t, float64(1), calls[1].Rate)
					require.ElementsMatch(t, append(expectedTags, "error:*errors.errorString"), calls[1].Tags)
				},
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				stats := &testutil.TestStatsClient{}
				defer func(old metrics.StatsClient) { metrics.Client = old }(metrics.Client)
				metrics.Client = stats

				// Wrap a test round-tripper with metrics
				rt := withMetrics(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					if tc.roundTripperError != nil {
						return nil, tc.roundTripperError
					}

					if req.Body != nil {
						if _, err := ioutil.ReadAll(req.Body); err != nil && err != io.EOF {
							return nil, err
						}
					}
					return &http.Response{}, nil
				}), tc.maxPayloadSize)
				_, err := rt.RoundTrip(&tc.req)
				if tc.expectedError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}

				tc.testRequestMetrics(t, stats)
			})
		}
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (r roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}
