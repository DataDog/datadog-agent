// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestBuildFormDataDataURIMimeType(t *testing.T) {
	datauri := "datauri"
	formCredentials := &privateconnection.PrivateCredentials{
		HttpDetails: privateconnection.HttpDetails{
			Body: privateconnection.HttpDetailsBody{
				Content:     "[]",
				ContentType: "multipart/form-data",
			},
		},
	}

	tests := []struct {
		name    string
		fields  []FormDataField
		wantErr string
	}{
		{
			name:   "type and subtype",
			fields: []FormDataField{{Key: "file", Value: "data:image/png;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:   "surrounding whitespace",
			fields: []FormDataField{{Key: "file", Value: "  data:text/plain;base64,aGVsbG8=\n", Type: &datauri}},
		},
		{
			name:   "subtype with extra slash",
			fields: []FormDataField{{Key: "file", Value: "data:application/vnd/x;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:   "empty subtype",
			fields: []FormDataField{{Key: "file", Value: "data:image/;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:   "empty type",
			fields: []FormDataField{{Key: "file", Value: "data:/png;base64,aGVsbG8=", Type: &datauri}},
		},
		{
			name:    "no slash",
			fields:  []FormDataField{{Key: "file", Value: "data:png;base64,aGVsbG8=", Type: &datauri}},
			wantErr: "Invalid mime type in data URI for file",
		},
		{
			name:    "empty mime type",
			fields:  []FormDataField{{Key: "file", Value: "data:;base64,aGVsbG8=", Type: &datauri}},
			wantErr: "Invalid mime type in data URI for file",
		},
		{
			name: "invalid field after valid ones",
			fields: []FormDataField{
				{Key: "name", Value: "report"},
				{Key: "file", Value: "data:image/png;base64,aGVsbG8=", Type: &datauri},
				{Key: "thumb", Value: "data:png;base64,aGVsbG8=", Type: &datauri},
			},
			wantErr: "Invalid mime type in data URI for thumb",
		},
	}

	for _, credentials := range []struct {
		name  string
		value *privateconnection.PrivateCredentials
	}{
		{name: "no connection", value: nil},
		{name: "connection form data", value: formCredentials},
	} {
		for _, tt := range tests {
			t.Run(credentials.name+"/"+tt.name, func(t *testing.T) {
				var (
					body        *bytes.Buffer
					contentType string
					err         error
				)
				require.NotPanics(t, func() {
					body, contentType, err = buildFormData(tt.fields, credentials.value)
				})
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					assert.Nil(t, body)
					return
				}
				require.NoError(t, err)
				require.NotNil(t, body)
				assert.Contains(t, contentType, "multipart/form-data")
				assert.Contains(t, body.String(), "aGVsbG8=")
			})
		}
	}
}

func TestHasSameDomain(t *testing.T) {
	for _, test := range []struct {
		name       string
		requestURL string
		baseURL    string
		want       bool
	}{
		{name: "https base, https request", requestURL: "https://example.com/path", baseURL: "https://example.com", want: true},
		{name: "http base, http request", requestURL: "http://example.com/path", baseURL: "http://example.com", want: true},
		{name: "http base, https request", requestURL: "https://example.com/path", baseURL: "http://example.com", want: true},
		{name: "uppercase https request", requestURL: "HTTPS://example.com/path", baseURL: "https://example.com", want: true},
		{name: "https base, http request", requestURL: "http://example.com/path", baseURL: "https://example.com", want: false},
		{name: "uppercase https base, uppercase http request", requestURL: "HTTP://example.com/path", baseURL: "HTTPS://example.com", want: false},
		{name: "https base with port, http request on same port", requestURL: "http://example.com:8443/path", baseURL: "https://example.com:8443", want: false},
		{name: "https base, scheme-relative request", requestURL: "//example.com/path", baseURL: "https://example.com", want: false},
		{name: "https base, other scheme request", requestURL: "ftp://example.com/path", baseURL: "https://example.com", want: false},
		{name: "different host", requestURL: "https://other.com/path", baseURL: "https://example.com", want: false},
		{name: "different port", requestURL: "https://example.com:8443/path", baseURL: "https://example.com", want: false},
		{name: "unparsable request", requestURL: "https://exa mple.com:bad/", baseURL: "https://example.com", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, hasSameDomain(test.requestURL, test.baseURL))
		})
	}
}

type staticClientProvider struct{ client httpclient.HTTPClient }

func (p staticClientProvider) NewDefaultClient() (httpclient.HTTPClient, error) { return p.client, nil }

func (p staticClientProvider) NewClient(*httpclient.RunnerHttpClientConfig) (httpclient.HTTPClient, error) {
	return p.client, nil
}

func TestRequestBaseURLScheme(t *testing.T) {
	for _, test := range []struct {
		name          string
		tlsServer     bool
		baseURLScheme string
		wantError     string
	}{
		{name: "https base rejects http request", baseURLScheme: "https", wantError: "specified URL does not match base URL configured in the Connection"},
		{name: "http base allows http request", baseURLScheme: "http"},
		{name: "http base allows https request", tlsServer: true, baseURLScheme: "http"},
		{name: "https base allows https request", tlsServer: true, baseURLScheme: "https"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusOK)
			})
			var server *httptest.Server
			if test.tlsServer {
				server = httptest.NewTLSServer(handler)
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			target, err := url.Parse(server.URL)
			require.NoError(t, err)

			task := &types.Task{}
			task.Data.Attributes = &types.Attributes{
				BundleID: "com.datadoghq.http", Name: "request",
				Inputs: map[string]interface{}{
					"verb": "GET",
					"url":  server.URL + "/path",
				},
			}
			credentials := &privateconnection.PrivateCredentials{
				Type: privateconnection.TokenAuthType,
				HttpDetails: privateconnection.HttpDetails{
					BaseURL: test.baseURLScheme + "://" + strings.SplitN(server.URL, "://", 2)[1],
				},
			}

			action := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second, Allowlist: []string{target.Hostname()}})
			// Trust the httptest certificate directly: allowExpiredCertificate still verifies chain and hostname.
			action.WithHttpClientProvider(staticClientProvider{client: server.Client()})
			_, err = action.Run(context.Background(), task, credentials)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Zero(t, requests.Load())
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}
