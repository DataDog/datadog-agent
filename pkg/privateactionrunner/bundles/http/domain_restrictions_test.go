// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestCredentialDomainRedirects(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, allowTarget := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/allowed=%t", status, allowTarget), func(t *testing.T) {
				var requests atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					assert.Equal(t, "secret-value", r.Header.Get("X-API-Key"))
					if status != http.StatusFound {
						body, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						assert.Contains(t, string(body), "secret-body")
					}
					_, _ = w.Write([]byte("ok"))
				}))
				defer destination.Close()
				target := strings.Replace(destination.URL, "127.0.0.1", "localhost", 1) + "/?token=secret-query"
				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, target, status)
				}))
				defer source.Close()
				domains := privateconnection.HTTPDomainAllowlist{"127.0.0.1"}
				if allowTarget {
					domains = append(domains, "localhost")
				}
				credentials := &privateconnection.PrivateCredentials{
					Type:                   privateconnection.TokenAuthType,
					HttpDetails:            privateconnection.HttpDetails{Headers: []privateconnection.PrivateCredentialsToken{{Name: "X-API-Key", Value: "secret-value"}}},
					HTTPDomainRestrictions: []privateconnection.HTTPDomainAllowlist{domains},
				}
				task := &types.Task{}
				task.Data.Attributes = &types.Attributes{Inputs: map[string]interface{}{
					"url": source.URL, "verb": "POST", "body": map[string]interface{}{"value": "secret-body"},
				}}
				// Exercise both IMDS-protected and explicitly unprotected clients.
				handler := NewHttpRequestAction(&config.Config{HTTPTimeout: time.Second * 5, AllowIMDSEndpoint: allowTarget})
				_, err := handler.Run(context.Background(), task, credentials)
				if allowTarget {
					require.NoError(t, err)
					assert.Equal(t, int32(1), requests.Load())
				} else {
					require.ErrorContains(t, err, "request domain is not allowed")
					assert.NotContains(t, err.Error(), "secret-query")
					assert.NotContains(t, err.Error(), "secret-value")
					assert.Equal(t, int32(0), requests.Load())
				}
			})
		}
	}
}

func TestHTTPActionsEnforceCredentialDomains(t *testing.T) {
	for _, action := range []string{"request", "testConnection", "getChecksumFromURL"} {
		for _, denial := range []string{"credential", "runner", "none"} {
			t.Run(action+"/"+denial, func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					_, _ = w.Write([]byte("ok"))
				}))
				defer server.Close()
				cfg := &config.Config{HTTPTimeout: 5 * time.Second}
				if denial == "runner" {
					cfg.Allowlist = []string{"other.example.com"}
				}
				domains := privateconnection.HTTPDomainAllowlist{"127.0.0.1"}
				if denial == "credential" {
					domains = privateconnection.HTTPDomainAllowlist{"other.example.com"}
				}
				credential := &privateconnection.PrivateCredentials{
					HTTPDomainRestrictions: []privateconnection.HTTPDomainAllowlist{domains},
					HttpDetails:            privateconnection.HttpDetails{BaseURL: server.URL, Testing: &privateconnection.HttpDetailsTesting{Verb: "GET", Path: "/health"}},
				}
				task := &types.Task{}
				task.Data.Attributes = &types.Attributes{Inputs: map[string]interface{}{"url": server.URL, "verb": "GET"}}
				_, err := NewHttpBundle(cfg).GetAction(action).Run(context.Background(), task, credential)
				if denial == "none" {
					require.NoError(t, err)
					assert.Equal(t, int32(1), requests.Load())
				} else {
					require.Error(t, err)
					assert.Equal(t, int32(0), requests.Load())
				}
			})
		}
	}
}
