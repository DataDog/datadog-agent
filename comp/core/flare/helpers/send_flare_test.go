// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestMkURL(t *testing.T) {
	assert.Equal(t, "https://example.com/support/flare/999", mkURL("https://example.com", "999"))
	assert.Equal(t, "https://example.com/support/flare", mkURL("https://example.com", ""))
}

func TestFlareHasRightForm(t *testing.T) {
	var lastRequest *http.Request

	cfg := config.NewMock(t)

	testCases := []struct {
		name        string
		handlerFunc http.HandlerFunc
		fail        bool
	}{
		{
			name: "ok",
			handlerFunc: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// This server serves two requests:
				//  * the original flare request URL, which redirects on HEAD to /post-target
				//  * HEAD /post-target - responds with 200 OK
				//  * POST /post-target - the final POST
				if r.Header.Get("DD-API-KEY") != "abcdef" {
					w.WriteHeader(403)
					io.WriteString(w, "request missing DD-API-KEY header")
				}

				if r.Method == "HEAD" && r.RequestURI == "/support/flare/12345" {
					// redirect to /post-target.
					w.Header().Set("Location", "/post-target")
					w.WriteHeader(307)
				} else if r.Method == "HEAD" && r.RequestURI == "/post-target" {
					// accept a HEAD to /post-target
					w.WriteHeader(200)
				} else if r.Method == "POST" && r.RequestURI == "/post-target" {
					// handle the actual POST
					w.Header().Set("Content-Type", "application/json")
					lastRequest = r
					err := lastRequest.ParseMultipartForm(1000000)
					assert.NoError(t, err)
					io.WriteString(w, "{}")
				} else {
					w.WriteHeader(500)
					io.WriteString(w, "path not recognized by httptest server")
				}
			}),
			fail: false,
		},
		{
			name: "service unavailable",
			handlerFunc: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(503)
				io.WriteString(w, "path not recognized by httptest server")
			}),
			fail: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(*testing.T) {
			ts := httptest.NewServer(testCase.handlerFunc)
			defer ts.Close()

			ddURL := ts.URL

			archivePath := "./test/blank.zip"
			caseID := "12345"
			email := "dev@datadoghq.com"
			apiKey := "abcdef"
			source := FlareSource{}

			_, err := SendTo(cfg, archivePath, caseID, email, apiKey, ddURL, source)

			if testCase.fail {
				assert.Error(t, err)
				expectedErrorMessage := "We couldn't reach the flare backend " +
					scrubber.ScrubLine(mkURL(ddURL, caseID)) +
					" via redirects: 503 Service Unavailable"
				assert.Equal(t, expectedErrorMessage, err.Error())
			} else {
				assert.NoError(t, err)
				av, _ := version.Agent()

				assert.Equal(t, caseID, lastRequest.FormValue("case_id"))
				assert.Equal(t, email, lastRequest.FormValue("email"))
				assert.Equal(t, av.String(), lastRequest.FormValue("agent_version"))
			}
		})
	}
}

func TestAnalyzeResponse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"case_id\": 1234}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.NoError(t, reserr)
		require.Equal(t,
			"Your logs were successfully uploaded. For future reference, your internal case id is 1234",
			resstr)
	})

	t.Run("error-from-server", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"case_id\": 1234, \"error\": \"uhoh\", \"request_uuid\": \"1dd9a912-843f-4987-9007-b915edb3d047\"}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t, errors.New("uhoh"), reserr)
		require.Equal(t,
			"An error occurred while uploading the flare: uhoh. Please contact support by email and facilitate the request uuid: `1dd9a912-843f-4987-9007-b915edb3d047`.",
			resstr)
	})

	t.Run("error-from-server-with-no-request_uuid", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"case_id\": 1234, \"error\": \"uhoh\"}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t, errors.New("uhoh"), reserr)
		require.Equal(t,
			"An error occurred while uploading the flare: uhoh. Please contact support by email.",
			resstr)
	})

	t.Run("unparseable-from-server", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json; charset=UTF-8"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("thats-not-json"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("invalid character 'h' in literal true (expecting 'r')\n"+
				"Server returned:\n"+
				"thats-not-json"),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("unparseable-from-server-huge", func(t *testing.T) {
		resp := "uhoh"
		for i := 0; i < 100; i++ {
			resp += "\npad this out to be pretty long"
		}
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte(resp))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("invalid character 'u' looking for beginning of value\n"+
				"Server returned:\n"+
				resp[:150]),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("no-content-type", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"json\": true}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("Server returned a 200 but with no content-type header\n"+
				"Server returned:\n"+
				"{\"json\": true}"),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("bad-content-type", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(bytes.NewBuffer([]byte("{\"json\": true}"))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("Server returned a 200 but with an unknown content-type text/plain\n"+
				"Server returned:\n"+
				"{\"json\": true}"),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("unknown-status", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 502,
			Status:     "Bad Gateway",
			Body:       io.NopCloser(bytes.NewBuffer([]byte("<html>.."))),
		}
		resstr, reserr := analyzeResponse(r, "abcdef")
		require.Equal(t,
			errors.New("HTTP 502 Bad Gateway\n"+
				"Server returned:\n"+
				"<html>.."),
			reserr)
		require.Equal(t,
			"Error: could not deserialize response body -- Please contact support by email.",
			resstr)
	})

	t.Run("forbidden-no-api-key", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 403,
		}
		resstr, reserr := analyzeResponse(r, "")
		require.Equal(t,
			errors.New("HTTP 403 Forbidden: API key is missing"),
			reserr)
		require.Equal(t, "", resstr)
	})

	t.Run("forbidden-with-api-key", func(t *testing.T) {
		r := &http.Response{
			StatusCode: 403,
		}
		resstr, reserr := analyzeResponse(r, "abcd123abcd12344abcd1234")
		require.Equal(t,
			errors.New("HTTP 403 Forbidden: Make sure your API key is valid. API Key ending with: d1234"),
			reserr)
		require.Equal(t, "", resstr)
	})
}

func TestSendToRetryLogic(t *testing.T) {
	cfg := config.NewMock(t)

	testCases := []struct {
		name             string
		serverBehavior   func(_ int) (statusCode int, response string, delay time.Duration)
		expectedAttempts int
		expectSuccess    bool
		expectedError    string
	}{
		{
			name: "success on first attempt",
			serverBehavior: func(_ int) (int, string, time.Duration) {
				return 200, `{"case_id": 1234}`, 0
			},
			expectedAttempts: 1,
			expectSuccess:    true,
		},
		{
			name: "success on second attempt after 500 error",
			serverBehavior: func(attempt int) (int, string, time.Duration) {
				if attempt == 1 {
					return 500, "Internal Server Error", 0
				}
				return 200, `{"case_id": 1234}`, 0
			},
			expectedAttempts: 2,
			expectSuccess:    true,
		},
		{
			name: "success on third attempt after 502 and 503 errors",
			serverBehavior: func(attempt int) (int, string, time.Duration) {
				switch attempt {
				case 1:
					return 502, "Bad Gateway", 0
				case 2:
					return 503, "Service Unavailable", 0
				default:
					return 200, `{"case_id": 1234}`, 0
				}
			},
			expectedAttempts: 3,
			expectSuccess:    true,
		},
		{
			name: "exhausted retries with 5xx errors",
			serverBehavior: func(_ int) (int, string, time.Duration) {
				return 500, "Internal Server Error", 0
			},
			expectedAttempts: 4, // 1 initial + 3 retries
			expectSuccess:    false,
			expectedError:    "failed to send flare after 4 attempts: server error: 500 Internal Server Error",
		},
		{
			name: "non-retryable 400 error",
			serverBehavior: func(_ int) (int, string, time.Duration) {
				return 400, "Bad Request", 0
			},
			expectedAttempts: 1,
			expectSuccess:    false,
			expectedError:    "HTTP 400 400 Bad Request",
		},
		{
			name: "non-retryable 404 error",
			serverBehavior: func(_ int) (int, string, time.Duration) {
				return 404, "Not Found", 0
			},
			expectedAttempts: 1,
			expectSuccess:    false,
			expectedError:    "HTTP 404 404 Not Found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attemptCount := 0
			var lastAttemptTime time.Time
			var timeBetweenAttempts []time.Duration

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("DD-API-KEY") != "test-api-key" {
					w.WriteHeader(403)
					io.WriteString(w, "Forbidden")
					return
				}

				if r.Method == "HEAD" && r.URL.Path == "/support/flare/12345" {
					w.Header().Set("Location", "/post-target")
					w.WriteHeader(307)
					return
				}
				if r.Method == "HEAD" && r.URL.Path == "/post-target" {
					w.WriteHeader(200)
					return
				}
				if r.Method == "POST" && r.URL.Path == "/post-target" {
					attemptCount++

					if !lastAttemptTime.IsZero() {
						timeBetweenAttempts = append(timeBetweenAttempts, time.Since(lastAttemptTime))
					}
					lastAttemptTime = time.Now()

					statusCode, response, delay := tc.serverBehavior(attemptCount)

					if delay > 0 {
						time.Sleep(delay)
					}

					if statusCode == 200 {
						w.Header().Set("Content-Type", "application/json")
					}
					w.WriteHeader(statusCode)
					io.WriteString(w, response)
					return
				}

				w.WriteHeader(404)
				io.WriteString(w, "Not Found")
			}))
			defer server.Close()
			result, err := SendTo(cfg, "./test/blank.zip", "12345", "test@example.com", "test-api-key", server.URL, FlareSource{})

			assert.Equal(t, tc.expectedAttempts, attemptCount, "Unexpected number of attempts")

			if tc.expectSuccess {
				assert.NoError(t, err, "Expected success but got error")
				assert.Contains(t, result, "Your logs were successfully uploaded", "Expected success message")
			} else {
				assert.Error(t, err, "Expected error but got success")
				if tc.expectedError != "" {
					assert.Contains(t, err.Error(), tc.expectedError, "Error message doesn't match expected")
				}
			}
			if len(timeBetweenAttempts) > 0 {
				for i, duration := range timeBetweenAttempts {
					expectedDelay := time.Duration(i+1) * 100 * time.Millisecond
					assert.True(t, duration >= expectedDelay-50*time.Millisecond && duration <= expectedDelay+50*time.Millisecond,
						"Retry delay %d was %v, expected around %v", i+1, duration, expectedDelay)
				}
			}
		})
	}
}

func TestIsRetryableFlareError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      errors.New("context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
			expected: true,
		},
		{
			name:     "connection refused error",
			err:      errors.New("dial tcp 127.0.0.1:8080: connection refused"),
			expected: true,
		},
		{
			name:     "connection reset error",
			err:      errors.New("read tcp 127.0.0.1:8080: connection reset by peer"),
			expected: true,
		},
		{
			name:     "network unreachable error",
			err:      errors.New("dial tcp 192.168.1.1:8080: network unreachable"),
			expected: true,
		},
		{
			name:     "temporary failure error",
			err:      errors.New("temporary failure in name resolution"),
			expected: true,
		},
		{
			name:     "non-retryable error",
			err:      errors.New("invalid request format"),
			expected: false,
		},
		{
			name:     "authentication error",
			err:      errors.New("authentication failed"),
			expected: false,
		},
		{
			name:     "validation error",
			err:      errors.New("validation failed"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryableFlareError(tc.err)
			assert.Equal(t, tc.expected, result, "isRetryableFlareError result mismatch")
		})
	}
}

func TestSendToWithNetworkErrors(t *testing.T) {
	cfg := config.NewMock(t)

	testCases := []struct {
		name      string
		errorType string
		errorMsg  string
	}{
		{
			name:      "connection refused error",
			errorType: "connection refused",
			errorMsg:  "connection refused",
		},
		{
			name:      "timeout error",
			errorType: "timeout",
			errorMsg:  "timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attemptCount := 0

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "HEAD" && r.URL.Path == "/support/flare/12345" {
					w.Header().Set("Location", "/post-target")
					w.WriteHeader(307)
					return
				}
				if r.Method == "HEAD" && r.URL.Path == "/post-target" {
					w.WriteHeader(200)
					return
				}
				if r.Method == "POST" && r.URL.Path == "/post-target" {
					attemptCount++
					// Simulate network error by closing connection
					if attemptCount <= 3 {
						// Close the connection to simulate network error
						w.Header().Set("Connection", "close")
						w.WriteHeader(500)
						return
					}
					// Success on 4th attempt
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					io.WriteString(w, `{"case_id": 1234}`)
				}
			}))
			defer server.Close()

			result, err := SendTo(cfg, "./test/blank.zip", "12345", "test@example.com", "test-api-key", server.URL, FlareSource{})

			// Should eventually succeed after retries
			assert.NoError(t, err, "Expected success after retries")
			assert.Contains(t, result, "Your logs were successfully uploaded", "Expected success message")
			assert.Equal(t, 4, attemptCount, "Expected 4 attempts before success")
		})
	}
}
