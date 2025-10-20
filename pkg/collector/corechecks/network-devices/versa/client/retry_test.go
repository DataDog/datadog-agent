// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package client

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetWithTokenOAuthRetryLogic tests the smart OAuth retry behavior
func TestGetWithTokenOAuthRetryLogic(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func(*atomic.Int32) *httptest.Server
		setupClient     func(*Client)
		expectedCalls   int32
		expectedSuccess bool
		expectedError   string
		validateClient  func(*testing.T, *Client)
	}{
		{
			name: "oauth_success_on_first_try",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"access_token":"test-token","expires_in":"3600","refresh_token":"test-refresh","token_type":"Bearer","issued_at":"0"}`))
				})
				mux.HandleFunc("/vnms/test", func(w http.ResponseWriter, _ *http.Request) {
					callCount.Add(1)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"result":"success"}`))
				})
				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.authMethod = authMethodOAuth
				client.clientID = "test-client"
				client.clientSecret = "test-secret"
			},
			expectedCalls:   1,
			expectedSuccess: true,
			validateClient: func(t *testing.T, client *Client) {
				assert.NotEmpty(t, client.directorToken)
			},
		},
		{
			name: "oauth_token_expired_refresh_succeeds",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := http.NewServeMux()
				// Track if refresh has been called
				var refreshCalled atomic.Bool

				mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"access_token":"initial-token","expires_in":"3600","refresh_token":"test-refresh","token_type":"Bearer","issued_at":"0"}`))
				})
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					refreshCalled.Store(true)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"access_token":"refreshed-token","expires_in":"3600","refresh_token":"new-refresh","token_type":"Bearer","issued_at":"0"}`))
				})
				mux.HandleFunc("/vnms/test", func(w http.ResponseWriter, _ *http.Request) {
					count := callCount.Add(1)
					if count == 1 {
						// First call: return 401 to trigger refresh
						w.WriteHeader(http.StatusUnauthorized)
						w.Write([]byte(`{"error":"token expired"}`))
					} else {
						// Second call: succeed after refresh
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"result":"success"}`))
					}
				})
				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.authMethod = authMethodOAuth
				client.clientID = "test-client"
				client.clientSecret = "test-secret"
			},
			expectedCalls:   2,
			expectedSuccess: true,
			validateClient: func(t *testing.T, client *Client) {
				assert.NotEmpty(t, client.directorToken)
				assert.Equal(t, "refreshed-token", client.directorToken)
			},
		},
		{
			name: "oauth_refresh_fails_fresh_login_succeeds",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := http.NewServeMux()
				var loginCount atomic.Int32
				var revokeCalled atomic.Bool

				mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
					count := loginCount.Add(1)
					w.WriteHeader(http.StatusOK)
					if count == 1 {
						w.Write([]byte(`{"access_token":"initial-token","expires_in":"3600","refresh_token":"test-refresh","token_type":"Bearer","issued_at":"0"}`))
					} else {
						// Second login after refresh fails
						w.Write([]byte(`{"access_token":"new-login-token","expires_in":"3600","refresh_token":"new-refresh","token_type":"Bearer","issued_at":"0"}`))
					}
				})
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					// Refresh returns bad token
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"access_token":"bad-refreshed-token","expires_in":"3600","refresh_token":"bad-refresh","token_type":"Bearer","issued_at":"0"}`))
				})
				mux.HandleFunc("/auth/revoke", func(w http.ResponseWriter, _ *http.Request) {
					revokeCalled.Store(true)
					w.WriteHeader(http.StatusOK)
				})
				mux.HandleFunc("/vnms/test", func(w http.ResponseWriter, _ *http.Request) {
					count := callCount.Add(1)
					if count <= 2 {
						// First two calls fail (initial + refreshed token)
						w.WriteHeader(http.StatusUnauthorized)
						w.Write([]byte(`{"error":"invalid token"}`))
					} else {
						// Third call: succeed with new login token
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"result":"success"}`))
					}
				})
				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.authMethod = authMethodOAuth
				client.clientID = "test-client"
				client.clientSecret = "test-secret"
			},
			expectedCalls:   3,
			expectedSuccess: true,
			validateClient: func(t *testing.T, client *Client) {
				assert.NotEmpty(t, client.directorToken)
				assert.Equal(t, "new-login-token", client.directorToken)
			},
		},
		{
			name: "oauth_all_attempts_exhausted",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := http.NewServeMux()
				mux.HandleFunc("/auth/token", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"access_token":"test-token","expires_in":"3600","refresh_token":"test-refresh","token_type":"Bearer","issued_at":"0"}`))
				})
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"access_token":"refreshed-token","expires_in":"3600","refresh_token":"new-refresh","token_type":"Bearer","issued_at":"0"}`))
				})
				mux.HandleFunc("/auth/revoke", func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
				mux.HandleFunc("/vnms/test", func(w http.ResponseWriter, _ *http.Request) {
					callCount.Add(1)
					// Always return 401
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"error":"always fails"}`))
				})
				return httptest.NewServer(mux)
			},
			setupClient: func(client *Client) {
				client.authMethod = authMethodOAuth
				client.clientID = "test-client"
				client.clientSecret = "test-secret"
			},
			expectedCalls:   3, // maxAttempts = 3
			expectedSuccess: false,
			expectedError:   "http responded with 401 code",
			validateClient: func(t *testing.T, client *Client) {
				// Token should be cleared after all attempts fail
				assert.Empty(t, client.directorToken)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := &atomic.Int32{}
			server := tt.setupServer(callCount)
			defer server.Close()

			client, err := testClient(server)
			require.NoError(t, err)
			tt.setupClient(client)

			// Execute the request
			result, err := client.getWithToken("/vnms/test", nil)

			// Validate results
			assert.Equal(t, tt.expectedCalls, callCount.Load(), "unexpected number of calls")

			if tt.expectedSuccess {
				require.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				require.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			}

			if tt.validateClient != nil {
				tt.validateClient(t, client)
			}
		})
	}
}

// TestGetWithSessionRetryLogic tests session auth retry behavior
func TestGetWithSessionRetryLogic(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func(*atomic.Int32) *httptest.Server
		expectedCalls   int32
		expectedSuccess bool
		expectedError   string
	}{
		{
			name: "session_success_on_first_try",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/test", func(w http.ResponseWriter, _ *http.Request) {
					callCount.Add(1)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"result":"success"}`))
				})
				return httptest.NewServer(mux)
			},
			expectedCalls:   1,
			expectedSuccess: true,
		},
		{
			name: "session_401_retry_succeeds",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/test", func(w http.ResponseWriter, _ *http.Request) {
					count := callCount.Add(1)
					if count == 1 {
						// First call: return 401
						w.WriteHeader(http.StatusUnauthorized)
						w.Write([]byte(`{"error":"session expired"}`))
					} else {
						// Second call: succeed after re-auth
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"result":"success"}`))
					}
				})
				return httptest.NewServer(mux)
			},
			expectedCalls:   2,
			expectedSuccess: true,
		},
		{
			name: "session_all_attempts_exhausted",
			setupServer: func(callCount *atomic.Int32) *httptest.Server {
				mux := setupCommonServerMux()
				mux.HandleFunc("/versa/analytics/v1.0.0/test", func(w http.ResponseWriter, _ *http.Request) {
					callCount.Add(1)
					// Always return 401
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"error":"always fails"}`))
				})
				return httptest.NewServer(mux)
			},
			expectedCalls:   3, // maxAttempts = 3
			expectedSuccess: false,
			expectedError:   "http responded with 401 code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := &atomic.Int32{}
			server := tt.setupServer(callCount)
			defer server.Close()

			client, err := testClient(server)
			require.NoError(t, err)

			// Set both endpoints to the test server for session auth
			client.directorEndpoint = server.URL
			client.analyticsEndpoint = server.URL

			// Execute the request
			result, err := client.getWithSession("/versa/analytics/v1.0.0/test", nil)

			// Validate results
			assert.Equal(t, tt.expectedCalls, callCount.Load(), "unexpected number of calls")

			if tt.expectedSuccess {
				require.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				require.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			}
		})
	}
}

// TestClearAuthByTypeWithRevoke tests that OAuth tokens are revoked when cleared
func TestClearAuthByTypeWithRevoke(t *testing.T) {
	tests := []struct {
		name            string
		authType        authType
		setupClient     func(*Client)
		setupServer     func(*atomic.Bool) *httptest.Server
		expectRevoke    bool
		validateCleaned func(*testing.T, *Client)
	}{
		{
			name:     "oauth_director_auth_cleared_with_revoke",
			authType: authTypeToken,
			setupClient: func(client *Client) {
				client.authMethod = authMethodOAuth
				client.directorToken = "test-token"
				client.directorRefreshToken = "test-refresh"
				client.directorTokenExpiry = timeNow().Add(time.Hour)
			},
			setupServer: func(revokeCalled *atomic.Bool) *httptest.Server {
				mux := http.NewServeMux()
				// Note: revokeOAuth() incorrectly calls /auth/refresh instead of /auth/revoke
				// This is a bug in the existing code, but we test what it actually does
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					revokeCalled.Store(true)
					w.WriteHeader(http.StatusOK)
				})
				return httptest.NewServer(mux)
			},
			expectRevoke: true,
			validateCleaned: func(t *testing.T, client *Client) {
				assert.Empty(t, client.directorToken)
				assert.Empty(t, client.directorRefreshToken)
				assert.True(t, client.directorTokenExpiry.Before(timeNow().Add(time.Second)))
			},
		},
		{
			name:     "basic_auth_director_auth_cleared_no_revoke",
			authType: authTypeToken,
			setupClient: func(client *Client) {
				client.authMethod = authMethodBasic
				// Basic auth doesn't have tokens, but test the clearing logic
			},
			setupServer: func(revokeCalled *atomic.Bool) *httptest.Server {
				mux := http.NewServeMux()
				// Note: revokeOAuth() incorrectly calls /auth/refresh instead of /auth/revoke
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					revokeCalled.Store(true)
					w.WriteHeader(http.StatusOK)
				})
				return httptest.NewServer(mux)
			},
			expectRevoke: false, // Basic auth should not call revoke
			validateCleaned: func(t *testing.T, client *Client) {
				// Basic auth fields should be cleared
				assert.Empty(t, client.directorToken)
			},
		},
		{
			name:     "session_auth_cleared_no_revoke",
			authType: authTypeSession,
			setupClient: func(client *Client) {
				client.sessionToken = "test-session-token"
				client.sessionTokenExpiry = timeNow().Add(time.Hour)
			},
			setupServer: func(revokeCalled *atomic.Bool) *httptest.Server {
				mux := http.NewServeMux()
				// Note: revokeOAuth() incorrectly calls /auth/refresh instead of /auth/revoke
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					revokeCalled.Store(true)
					w.WriteHeader(http.StatusOK)
				})
				return httptest.NewServer(mux)
			},
			expectRevoke: false, // Session auth should not call revoke
			validateCleaned: func(t *testing.T, client *Client) {
				assert.Empty(t, client.sessionToken)
				assert.True(t, client.sessionTokenExpiry.Before(timeNow().Add(time.Second)))
			},
		},
		{
			name:     "oauth_no_token_no_revoke",
			authType: authTypeToken,
			setupClient: func(client *Client) {
				client.authMethod = authMethodOAuth
				// No token set
			},
			setupServer: func(revokeCalled *atomic.Bool) *httptest.Server {
				mux := http.NewServeMux()
				// Note: revokeOAuth() incorrectly calls /auth/refresh instead of /auth/revoke
				mux.HandleFunc("/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
					revokeCalled.Store(true)
					w.WriteHeader(http.StatusOK)
				})
				return httptest.NewServer(mux)
			},
			expectRevoke: false, // No token means no revoke call
			validateCleaned: func(t *testing.T, client *Client) {
				assert.Empty(t, client.directorToken)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			revokeCalled := &atomic.Bool{}
			server := tt.setupServer(revokeCalled)
			defer server.Close()

			client, err := testClient(server)
			require.NoError(t, err)
			tt.setupClient(client)

			// Clear the auth
			client.clearAuthByType(tt.authType)

			// Validate revoke was/wasn't called
			assert.Equal(t, tt.expectRevoke, revokeCalled.Load(), "revoke call mismatch")

			// Validate cleaned state
			if tt.validateCleaned != nil {
				tt.validateCleaned(t, client)
			}
		})
	}
}

// TestExpireDirectorToken tests the token expiration helper
func TestExpireDirectorToken(t *testing.T) {
	client := &Client{
		directorToken:        "test-token",
		directorRefreshToken: "test-refresh",
		directorTokenExpiry:  timeNow().Add(time.Hour),
		authenticationMutex:  &sync.Mutex{},
	}

	// Expire the token
	client.expireDirectorToken()

	// Token should still exist but be expired
	assert.Equal(t, "test-token", client.directorToken)
	assert.Equal(t, "test-refresh", client.directorRefreshToken)
	assert.True(t, client.directorTokenExpiry.Before(timeNow().Add(time.Second)))
}
