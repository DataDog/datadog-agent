// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opms

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/stretchr/testify/require"
)

func TestPhoneHomePOSTRefusesRedirectAndDoesNotRetry(t *testing.T) {
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("enrollment credential forwarded across redirect")
	}))
	defer target.Close()
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
		}))
		client := NewPublicClient(configmock.New(t), srv.URL, nil).(*publicClient)
		_, err := client.doEnrollRequestWithRetry(context.Background(), srv.URL, []byte(`{}`), "secret", "")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
		require.Equal(t, 1, calls)
		srv.Close()
	}
	t.Setenv(app.PhoneHomePOCEnvVar, "false")
	client := NewPublicClient(configmock.New(t), "https://api.datadoghq.com", nil).(*publicClient)
	require.False(t, client.phoneHomePOC)
	require.Nil(t, client.httpClient.CheckRedirect, "default behavior remains unchanged")
}
