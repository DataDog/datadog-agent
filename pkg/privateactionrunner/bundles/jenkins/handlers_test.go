// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type jenkinsHandler interface {
	Run(ctx context.Context, task *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error)
}

func newJenkinsHandlers(cfg *config.Config) map[string]jenkinsHandler {
	return map[string]jenkinsHandler{
		"buildJob":     NewBuildJobHandler(cfg),
		"deleteJob":    NewDeleteJobHandler(cfg),
		"getJobStatus": NewGetJobStatusHandler(cfg),
	}
}

func newJenkinsTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.jenkins", Inputs: inputs}
	return task
}

func newJenkinsCredentials(domain string) *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "username", Value: "user"},
			{Name: "token", Value: "api-token"},
			{Name: "domain", Value: domain},
		},
	}
}

func TestHandlersRejectRedirectsOutsideDomain(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		otherHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"displayName":"x","duration":0,"estimatedDuration":0,"id":"1","building":false,"result":"SUCCESS","url":"x"}`))
	}))
	defer other.Close()

	jenkins := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/elsewhere", http.StatusFound)
	}))
	defer jenkins.Close()

	for name, handler := range newJenkinsHandlers(defaultConfig) {
		t.Run(name, func(t *testing.T) {
			_, err := handler.Run(context.Background(), newJenkinsTask(map[string]interface{}{"jobName": "my-job"}), newJenkinsCredentials(jenkins.URL))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "different origin")
		})
	}
	assert.Zero(t, otherHits.Load(), "no request may reach an origin other than the connection domain")
}

// defaultConfig mirrors the defaults: http_allowlist is empty, which denies
// every HTTP action request, and domain_allowlist is unset, which allows every
// domain. Jenkins actions must keep working under it.
var defaultConfig = &config.Config{Allowlist: []string{}}

func TestHandlersEnforceConfiguredDomainAllowlist(t *testing.T) {
	var hits atomic.Int32
	jenkins := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer jenkins.Close()

	for _, allowlist := range [][]string{{}, {"jenkins.example.com"}, {"*.example.com"}} {
		for name, handler := range newJenkinsHandlers(&config.Config{Allowlist: []string{"*"}, DomainAllowlist: allowlist}) {
			t.Run(fmt.Sprintf("%s/%v", name, allowlist), func(t *testing.T) {
				_, err := handler.Run(context.Background(), newJenkinsTask(map[string]interface{}{"jobName": "my-job"}), newJenkinsCredentials(jenkins.URL))
				require.Error(t, err)
				assert.Contains(t, err.Error(), "private_action_runner.domain_allowlist")
			})
		}
	}
	assert.Zero(t, hits.Load(), "a domain outside a configured domain_allowlist must not be requested")
}

func TestHandlersReachConfiguredDomainAllowlistDomain(t *testing.T) {
	jenkins := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer jenkins.Close()

	cfg := &config.Config{Allowlist: []string{}, DomainAllowlist: []string{"127.0.0.1"}}
	_, err := NewBuildJobHandler(cfg).Run(context.Background(), newJenkinsTask(map[string]interface{}{"jobName": "my-job"}), newJenkinsCredentials(jenkins.URL))
	require.NoError(t, err)
}

func TestGetJobStatusFollowsRedirectWithinDomain(t *testing.T) {
	jenkins := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/job/my-job/lastBuild/api/json":
			http.Redirect(w, r, "/job/my-job/7/api/json", http.StatusFound)
		case "/job/my-job/7/api/json":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"displayName":"#7","duration":12,"estimatedDuration":15,"id":"7","building":true,"result":"","url":"u"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer jenkins.Close()

	out, err := NewGetJobStatusHandler(defaultConfig).Run(
		context.Background(),
		newJenkinsTask(map[string]interface{}{"jobName": "my-job"}),
		newJenkinsCredentials(jenkins.URL),
	)
	require.NoError(t, err)
	assert.Equal(t, &GetJobStatusOutputs{
		DisplayName:       "#7",
		Duration:          12,
		EstimatedDuration: 15,
		ID:                "7",
		InProgress:        true,
		URL:               "u",
	}, out)
}

func TestBuildAndDeleteJobIgnoreHTTPAllowlist(t *testing.T) {
	var buildHits, deleteHits atomic.Int32
	jenkins := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/job/my-job/build":
			buildHits.Add(1)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && r.URL.Path == "/job/my-job/":
			deleteHits.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer jenkins.Close()

	cfg := defaultConfig
	task := newJenkinsTask(map[string]interface{}{"jobName": "my-job"})
	_, err := NewBuildJobHandler(cfg).Run(context.Background(), task, newJenkinsCredentials(jenkins.URL))
	require.NoError(t, err)
	_, err = NewDeleteJobHandler(cfg).Run(context.Background(), task, newJenkinsCredentials(jenkins.URL))
	require.NoError(t, err)
	assert.Equal(t, int32(1), buildHits.Load())
	assert.Equal(t, int32(1), deleteHits.Load())
}
