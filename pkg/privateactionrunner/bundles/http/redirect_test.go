// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func redirectingServers(t *testing.T) (origin, elsewhere *httptest.Server, followed *bool) {
	followed = new(bool)
	elsewhere = httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { *followed = true }))
	t.Cleanup(elsewhere.Close)
	origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same-host" {
			*followed = true
			return
		}
		// Another hostname, since both servers listen on 127.0.0.1.
		target := strings.Replace(elsewhere.URL, "127.0.0.1", "localhost", 1)
		if r.URL.Query().Get("to") == "same" {
			target = "/same-host"
		}
		http.Redirect(w, r, target, http.StatusFound)
	}))
	t.Cleanup(origin.Close)
	return origin, elsewhere, followed
}

// followRedirect: false used to be checked only after the redirect had been
// followed, so the runner still contacted the redirect target.
func TestRequestFollowRedirectFalseDoesNotContactTarget(t *testing.T) {
	origin, _, followed := redirectingServers(t)
	handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second})

	_, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": origin.URL, "followRedirect": false}), nil)
	require.ErrorContains(t, err, "Enable 'Follow redirect'")
	assert.False(t, *followed)
}

// A followed redirect gets the same checks as the first URL: the runner
// allowlist, and the connection's base URL.
func TestRequestRedirectHopsAreChecked(t *testing.T) {
	origin, _, followed := redirectingServers(t)
	for _, test := range []struct {
		name       string
		allowlist  []string
		credential *privateconnection.PrivateCredentials
		query      string
		wantError  string
	}{
		{name: "outside allowlist", allowlist: []string{"127.0.0.1"}, wantError: "redirect url is not allowed by runner policy"},
		{name: "outside base URL", credential: &privateconnection.PrivateCredentials{Type: privateconnection.TokenAuthType, HttpDetails: privateconnection.HttpDetails{BaseURL: origin.URL}}, wantError: "redirect url does not match base URL"},
		{name: "same host allowed", credential: &privateconnection.PrivateCredentials{Type: privateconnection.TokenAuthType, HttpDetails: privateconnection.HttpDetails{BaseURL: origin.URL}}, query: "?to=same"},
		{name: "no restriction", allowlist: []string{"127.0.0.1", "localhost"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			*followed = false
			cfg := &config.Config{HTTPTimeout: 5 * time.Second, Allowlist: test.allowlist}
			_, err := NewHttpRequestAction(cfg).Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": origin.URL + test.query}), test.credential)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.False(t, *followed)
				return
			}
			require.NoError(t, err)
			assert.True(t, *followed)
		})
	}
}

// Raw-mode inputs must not change the default mode silently.
func TestRequestRejectsRawInputsInDefaultMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("no request expected") }))
	defer server.Close()
	for name, inputs := range map[string]map[string]interface{}{
		"bodyBase64":           {"verb": "POST", "url": server.URL, "bodyBase64": "YQ=="},
		"encryptResponseTo":    {"verb": "GET", "url": server.URL, "encryptResponseTo": map[string]interface{}{"keyType": RawResponseKeyType, "publicKey": "AA=="}},
		"encryptedCredentials": {"verb": "GET", "url": server.URL, "encryptedCredentials": "AA=="},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second}).Run(context.Background(), requestTask(inputs), nil)
			require.ErrorContains(t, err, "require responseMode raw")
		})
	}
}
