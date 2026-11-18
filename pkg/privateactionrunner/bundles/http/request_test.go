// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func newMethodRecordingServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), methods...)
	}
}

func newHTTPTestConfig(t *testing.T, serverURL string) *config.Config {
	t.Helper()
	target, err := url.Parse(serverURL)
	require.NoError(t, err)
	return &config.Config{HTTPTimeout: 5 * time.Second, Allowlist: []string{target.Hostname()}}
}

func newRequestTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.http",
		Name:     "request",
		Inputs:   inputs,
	}
	return task
}

func TestRequestVerb(t *testing.T) {
	for _, test := range []struct {
		name       string
		verb       interface{}
		wantMethod string
		wantError  string
	}{
		{name: "HEAD", verb: "HEAD", wantMethod: http.MethodHead},
		{name: "GET", verb: "GET", wantMethod: http.MethodGet},
		{name: "PUT", verb: "PUT", wantMethod: http.MethodPut},
		{name: "POST", verb: "POST", wantMethod: http.MethodPost},
		{name: "DELETE", verb: "DELETE", wantMethod: http.MethodDelete},
		{name: "PATCH", verb: "PATCH", wantMethod: http.MethodPatch},
		{name: "lowercase is canonicalized", verb: "patch", wantMethod: http.MethodPatch},
		{name: "mixed case is canonicalized", verb: "Delete", wantMethod: http.MethodDelete},
		{name: "empty defaults to GET", verb: "", wantMethod: http.MethodGet},
		{name: "missing defaults to GET", wantMethod: http.MethodGet},
		{name: "TRACE is rejected", verb: "TRACE", wantError: `unsupported HTTP verb "TRACE"`},
		{name: "OPTIONS is rejected", verb: "OPTIONS", wantError: `unsupported HTTP verb "OPTIONS"`},
		{name: "CONNECT is rejected", verb: "CONNECT", wantError: `unsupported HTTP verb "CONNECT"`},
		{name: "extension method is rejected", verb: "PROPFIND", wantError: `unsupported HTTP verb "PROPFIND"`},
		{name: "lowercase extension method is rejected", verb: "mkcol", wantError: `unsupported HTTP verb "mkcol"`},
		{name: "padded verb is rejected", verb: " GET", wantError: `unsupported HTTP verb " GET"`},
		{name: "verb with suffix is rejected", verb: "GETX", wantError: `unsupported HTTP verb "GETX"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, methods := newMethodRecordingServer(t)
			inputs := map[string]interface{}{"url": server.URL + "/resource"}
			if test.verb != nil {
				inputs["verb"] = test.verb
			}
			handler := NewHttpRequestAction(newHTTPTestConfig(t, server.URL))

			output, err := handler.Run(context.Background(), newRequestTask(inputs), nil)

			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, output)
				require.Empty(t, methods())
				return
			}
			require.NoError(t, err)
			response, ok := output.(*RequestOutputs)
			require.True(t, ok)
			require.Equal(t, http.StatusOK, response.Status)
			require.Equal(t, []string{test.wantMethod}, methods())
		})
	}
}

func TestRequestVerbRejectedWithBodyAndFormData(t *testing.T) {
	for _, test := range []struct {
		name   string
		inputs map[string]interface{}
	}{
		{name: "body", inputs: map[string]interface{}{"body": map[string]interface{}{"key": "value"}}},
		{name: "form data", inputs: map[string]interface{}{"formData": []interface{}{map[string]interface{}{"key": "k", "value": "v"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, methods := newMethodRecordingServer(t)
			test.inputs["verb"] = "PROPPATCH"
			test.inputs["url"] = server.URL + "/resource"
			handler := NewHttpRequestAction(newHTTPTestConfig(t, server.URL))

			_, err := handler.Run(context.Background(), newRequestTask(test.inputs), nil)

			require.ErrorContains(t, err, `unsupported HTTP verb "PROPPATCH"`)
			require.Empty(t, methods())
		})
	}
}

func TestTestConnectionVerb(t *testing.T) {
	for _, test := range []struct {
		name       string
		verb       string
		wantMethod string
		wantError  string
	}{
		{name: "configured verb", verb: "POST", wantMethod: http.MethodPost},
		{name: "empty defaults to GET", verb: "", wantMethod: http.MethodGet},
		{name: "unsupported verb is rejected", verb: "TRACE", wantError: `unsupported HTTP verb "TRACE"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, methods := newMethodRecordingServer(t)
			credential := &privateconnection.PrivateCredentials{
				HttpDetails: privateconnection.HttpDetails{
					BaseURL: server.URL,
					Testing: &privateconnection.HttpDetailsTesting{Path: "/health", Verb: test.verb},
				},
			}
			task := newRequestTask(nil)
			task.Data.Attributes.Name = "testConnection"
			handler := NewTestConnectionHandler(newHTTPTestConfig(t, server.URL))

			_, err := handler.Run(context.Background(), task, credential)

			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Empty(t, methods())
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{test.wantMethod}, methods())
		})
	}
}
