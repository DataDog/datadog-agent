// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

package lite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/stretchr/testify/require"
)

func TestRescueResolvesOnlyReportingSecrets(t *testing.T) {
	cleanEnv(t)
	requestFile := filepath.Join(t.TempDir(), "request.json")
	command := filepath.Join(t.TempDir(), "backend")
	require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\ncat > \"$1\"\nprintf '%s' '{\"key\":{\"value\":\"resolved-sentinel\"}}'\n"), 0700))
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- r.Header.Get("DD-API-KEY") + "\n" + string(body)
	}))
	defer server.Close()
	p := Params{ConfigPath: configFile(t, fmt.Sprintf("api_key: ENC[key]\ndd_url: %s\nsecret_backend_command: %s\nsecret_backend_arguments: [%q]\nlogs_config:\n  unrelated: ENC[unavailable]\n", server.URL, command, requestFile))}
	require.NoError(t, Rescue(context.Background(), p, errors.New("startup failed: resolved-sentinel ENC[key]")))
	select {
	case request := <-requests:
		require.Contains(t, request, "resolved-sentinel\n")
		require.NotContains(t, request[len("resolved-sentinel\n"):], "sentinel")
		require.NotContains(t, request, "ENC[key]")
	default:
		t.Fatal("ENC-backed startup report was not delivered")
	}
	raw, err := os.ReadFile(requestFile)
	require.NoError(t, err)
	var payload struct {
		Version string   `json:"version"`
		Secrets []string `json:"secrets"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, secrets.PayloadVersion, payload.Version)
	require.Equal(t, []string{"key"}, payload.Secrets, "unrelated secrets must not reach the helper")
	// The real resolver must still reject unsafe executable permissions.
	require.NoError(t, os.Chmod(command, 0777))
	require.Error(t, Rescue(context.Background(), p, errors.New("failed")))
	require.Empty(t, requests)
}

func TestRescueRedactsOverriddenCredentials(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DD_API_KEY", "env-sentinel")
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
	}))
	defer server.Close()
	p := Params{ConfigPath: configFile(t, "api_key: file-sentinel\ndd_url: "+server.URL+"\nsecret_backend_config:\n  token: backend-sentinel\n")}
	require.NoError(t, Rescue(context.Background(), p, errors.New("failure mentions file-sentinel env-sentinel backend-sentinel")))
	require.NotContains(t, <-received, "sentinel")
}

func TestRescueFleetSecretPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, key, route, wantKey, wantRoute string
		env, disabled, encryptedFleet        bool
	}{
		{name: "main secrets", key: "ENC[key]", route: "ENC[url]", wantKey: "resolved-key", wantRoute: "main"},
		{name: "env secrets", key: "plain-key", route: "main", env: true, wantKey: "resolved-key", wantRoute: "main"},
		{name: "plaintext Fleet wins", key: "plain-key", route: "main", wantKey: "fleet-key", wantRoute: "fleet"},
		{name: "trimmed main key", key: " plain-key ", route: "main", wantKey: "plain-key", wantRoute: "fleet"},
		{name: "Fleet opt out", key: "ENC[key]", route: "ENC[url]", disabled: true},
		{name: "no second Fleet resolution", key: "plain-key", route: "main", encryptedFleet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			requests := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path + " " + r.Header.Get("DD-API-KEY")
			}))
			defer server.Close()
			command := filepath.Join(t.TempDir(), "backend")
			marker := filepath.Join(t.TempDir(), "calls")
			response := fmt.Sprintf(`{"key":{"value":"resolved-key"},"url":{"value":%q}}`, server.URL+"/main")
			require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\ncat >/dev/null\nprintf x >> \"$1\"\nprintf '%s' '"+response+"'\n"), 0700))
			route := tc.route
			if route == "main" {
				route = server.URL + "/main"
			}
			main := configFile(t, fmt.Sprintf("api_key: %q\ndd_url: %q\nsecret_backend_command: %q\nsecret_backend_arguments: [%q]\n", tc.key, route, command, marker))
			fleetKey, fleetURL := "fleet-key", server.URL+"/fleet"
			if tc.encryptedFleet {
				fleetKey, fleetURL = "ENC[key]", "ENC[url]"
			}
			fleet := configFile(t, fmt.Sprintf("api_key: %q\ndd_url: %q\nhealth_platform.enabled: %t\n", fleetKey, fleetURL, !tc.disabled))
			if tc.env {
				t.Setenv("DD_API_KEY", "ENC[key]")
				t.Setenv("DD_DD_URL", "ENC[url]")
			}
			err := Rescue(context.Background(), Params{ConfigPath: main, FleetPoliciesDir: filepath.Dir(fleet)}, errors.New("failed"))
			if tc.encryptedFleet {
				require.Error(t, err)
				require.NoFileExists(t, marker)
			} else {
				require.NoError(t, err)
			}
			if tc.wantKey == "" {
				require.Empty(t, requests)
			} else {
				require.Len(t, requests, 1)
				require.Equal(t, "/"+tc.wantRoute+"/api/v2/agenthealth "+tc.wantKey, <-requests)
			}
		})
	}
}

func TestRescueSanitizesAPIKey(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		t.Run(strconv.FormatBool(encrypted), func(t *testing.T) {
			cleanEnv(t)
			requests := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests <- r.Header.Get("DD-API-KEY") + "\n" + string(body)
			}))
			defer server.Close()
			const rawKey = "  key-sentinel \n\t"
			t.Setenv("DD_API_KEY", rawKey)
			raw := "dd_url: " + server.URL + "\n"
			if encrypted {
				t.Setenv("DD_API_KEY", "ENC[key]")
				command := filepath.Join(t.TempDir(), "backend")
				response, err := json.Marshal(map[string]map[string]string{"key": {"value": rawKey}})
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' '"+string(response)+"'\n"), 0700))
				raw += fmt.Sprintf("secret_backend_command: %q\n", command)
			}
			err := Rescue(context.Background(), Params{ConfigPath: configFile(t, raw)}, errors.New("failure: "+rawKey+" normalized key-sentinel"))
			require.NoError(t, err)
			require.Len(t, requests, 1)
			request := <-requests
			require.Contains(t, request, "key-sentinel\n")
			require.NotContains(t, request[len("key-sentinel\n"):], "sentinel")
		})
	}
}
