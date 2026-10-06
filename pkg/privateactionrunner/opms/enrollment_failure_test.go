// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opms

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/stretchr/testify/require"
)

func TestEnrollmentFailureContract(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		code             int
		title, category  string
		retry, reconcile bool
	}{
		{403, "your organization has reached the maximum number of private action runners", "quota", true, false},
		{403, "required scope missing", "missing_scope", true, false},
		{401, "invalid auth context", "invalid_credentials", true, false},
		{400, "error decoding request", "invalid_request", false, false},
		{400, "missing required field", "invalid_request", false, false},
		{400, "value not allowed", "invalid_request", false, false},
		{400, "a runner with this name already exists", "name_conflict", false, true},
		{403, "quota-ish prose is not a contract", "forbidden_unknown", false, true},
		{500, "required scope missing", "response_ambiguous", false, true},
	} {
		t.Run(tc.category+tc.title, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"errors":[{"status":"%d","title":%q,"detail":"secret"}]}`, tc.code, tc.title))
			failure := responseFailure(tc.code, body, "", now)
			require.Equal(t, tc.category, failure.Category)
			require.Equal(t, tc.retry, failure.RetrySafe)
			require.Equal(t, tc.reconcile, failure.ReconciliationRequired)
			require.NotContains(t, failure.Error(), "secret")
		})
	}
	for _, body := range []string{"", `{"errors":["required scope missing"]}`, `{"errors":[{"status":"401","title":"required scope missing"}]}`, `{"errors":[{"status":"403","title":"required scope missing"},{"status":"403","title":"unknown"}]}`} {
		require.False(t, responseFailure(403, []byte(body), "", now).RetrySafe)
	}
	for _, tc := range []struct {
		header string
		delay  time.Duration
	}{{"3600", time.Hour}, {now.Add(time.Hour).Format(http.TimeFormat), time.Hour}, {"", 5 * time.Minute}, {"invalid", 5 * time.Minute}, {"-1", 5 * time.Minute}} {
		failure := responseFailure(429, nil, tc.header, now)
		require.Equal(t, now.Add(tc.delay), failure.RetryAt)
		require.False(t, failure.RetrySafe, "timing alone does not authorize a POST replay")
		require.True(t, failure.ReconciliationRequired)
		quota := responseFailure(403, []byte(`{"errors":[{"status":"403","title":"your organization has reached the maximum number of private action runners"}]}`), tc.header, now)
		require.True(t, quota.RetrySafe)
		if tc.header == "" {
			require.True(t, quota.RetryAt.IsZero())
		} else {
			require.True(t, quota.RetryAt.Equal(now.Add(tc.delay)))
		}
	}
}

func TestThrottleRetainsRetryAfterWithoutReadingBody(t *testing.T) {
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	deadline := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprintf("oversized_%t", oversized), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", deadline.Format(http.TimeFormat))
				if !oversized {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(http.StatusTooManyRequests)
				body := "truncated"
				if oversized {
					body = strings.Repeat("x", (1<<20)+1)
				}
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			client := NewPublicClient(configmock.New(t), srv.URL, nil).(*publicClient)
			_, _, err := client.doEnrollRequest(context.Background(), srv.URL, []byte(`{}`), "key", "")
			var failure *EnrollmentFailure
			require.ErrorAs(t, err, &failure)
			require.Equal(t, "throttled_unverified", failure.Category)
			require.True(t, deadline.Equal(failure.RetryAt))
			require.False(t, failure.RetrySafe)
			require.True(t, failure.ReconciliationRequired)
		})
	}
}

func TestEnrollmentCancellationAfterSubmissionIsAmbiguous(t *testing.T) {
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	client := NewPublicClient(configmock.New(t), srv.URL, nil).(*publicClient)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := client.doEnrollRequest(ctx, srv.URL, []byte(`{}`), "key", ""); done <- err }()
	<-entered
	cancel()
	var err error
	select {
	case err = <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
	var failure *EnrollmentFailure
	require.ErrorAs(t, err, &failure)
	require.False(t, failure.RetrySafe)
	require.True(t, failure.ReconciliationRequired)
	_, _, err = client.doEnrollRequest(ctx, srv.URL, []byte(`{}`), "key", "")
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "not_submitted", failure.Category)
	require.True(t, failure.RetrySafe)
	dial := transportFailure(&net.OpError{Op: "dial", Err: errors.New("connection refused")})
	require.True(t, dial.RetrySafe)
	require.False(t, transportFailure(&net.OpError{Op: "read", Err: errors.New("EOF")}).RetrySafe)
}
