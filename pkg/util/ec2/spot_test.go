// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func resetSpotTestVars() {
	instanceLifeCycleFetcher.Reset()
	resetPackageVars()
}

func TestIsSpotInstance(t *testing.T) {
	ctx := context.Background()
	configmock.New(t)

	tests := []struct {
		name         string
		lifecycle    string
		responseCode int
		expected     bool
		expectError  bool
	}{
		{
			name:         "spot instance",
			lifecycle:    "spot",
			responseCode: http.StatusOK,
			expected:     true,
			expectError:  false,
		},
		{
			name:         "on-demand instance",
			lifecycle:    "on-demand",
			responseCode: http.StatusOK,
			expected:     false,
			expectError:  false,
		},
		{
			name:         "scheduled instance",
			lifecycle:    "scheduled",
			responseCode: http.StatusOK,
			expected:     false,
			expectError:  false,
		},
		{
			name:         "IMDS error",
			lifecycle:    "",
			responseCode: http.StatusNotFound,
			expected:     false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPut:
					io.WriteString(w, testIMDSToken)
				case http.MethodGet:
					if tt.responseCode != http.StatusOK {
						w.WriteHeader(tt.responseCode)
						return
					}
					io.WriteString(w, tt.lifecycle)
				}
			}))
			defer ts.Close()

			ec2internal.MetadataURL = ts.URL
			ec2internal.TokenURL = ts.URL
			ec2internal.Token = httputils.NewAPIToken(ec2internal.GetToken)
			defer resetSpotTestVars()

			isSpot, err := IsSpotInstance(ctx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, isSpot)
			}
		})
	}
}

func TestGetSpotTerminationTime(t *testing.T) {
	ctx := context.Background()
	configmock.New(t)

	tests := []struct {
		name               string
		instanceLifeCycle  string
		spotActionCode     int
		spotActionBody     string
		expectedTime       time.Time
		expectError        bool
		expectNotSpotError bool
		errorContains      string
	}{
		{
			name:              "valid termination notice",
			instanceLifeCycle: "spot",
			spotActionCode:    http.StatusOK,
			spotActionBody:    `{"action": "terminate", "time": "2017-09-18T08:22:00Z"}`,
			expectedTime:      time.Date(2017, 9, 18, 8, 22, 0, 0, time.UTC),
			expectError:       false,
		},
		{
			name:              "valid termination notice with stop action",
			instanceLifeCycle: "spot",
			spotActionCode:    http.StatusOK,
			spotActionBody:    `{"action": "stop", "time": "2024-01-15T12:30:00Z"}`,
			expectedTime:      time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC),
			expectError:       false,
		},
		{
			name:              "no spot termination scheduled (404)",
			instanceLifeCycle: "spot",
			spotActionCode:    http.StatusNotFound,
			spotActionBody:    "",
			expectError:       true,
			errorContains:     "unable to retrieve spot instance-action",
		},
		{
			name:              "invalid JSON response",
			instanceLifeCycle: "spot",
			spotActionCode:    http.StatusOK,
			spotActionBody:    `not valid json`,
			expectError:       true,
			errorContains:     "unable to parse spot instance-action response",
		},
		{
			name:              "invalid time format",
			instanceLifeCycle: "spot",
			spotActionCode:    http.StatusOK,
			spotActionBody:    `{"action": "terminate", "time": "invalid-time"}`,
			expectError:       true,
			errorContains:     "unable to parse termination time",
		},
		{
			name:               "not a spot instance",
			instanceLifeCycle:  "on-demand",
			spotActionCode:     http.StatusOK,
			spotActionBody:     `{"action": "terminate", "time": "2017-09-18T08:22:00Z"}`,
			expectError:        true,
			expectNotSpotError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodPut:
					// Token request
					io.WriteString(w, testIMDSToken)
				case http.MethodGet:
					// Route based on URL path
					if strings.HasSuffix(r.URL.Path, "/instance-life-cycle") {
						io.WriteString(w, tt.instanceLifeCycle)
					} else if strings.HasSuffix(r.URL.Path, "/spot/instance-action") {
						if tt.spotActionCode != http.StatusOK {
							w.WriteHeader(tt.spotActionCode)
							return
						}
						io.WriteString(w, tt.spotActionBody)
					}
				}
			}))
			defer ts.Close()

			ec2internal.MetadataURL = ts.URL
			ec2internal.TokenURL = ts.URL
			ec2internal.Token = httputils.NewAPIToken(ec2internal.GetToken)
			defer resetSpotTestVars()

			terminationTime, err := GetSpotTerminationTime(ctx)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectNotSpotError {
					assert.True(t, errors.Is(err, ErrNotSpotInstance), "expected ErrNotSpotInstance, got: %v", err)
				} else {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedTime, terminationTime)
			}
		})
	}
}
