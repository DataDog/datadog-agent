// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/stretchr/testify/require"
)

func TestStandaloneEnrollmentUsesExistingStartupPath(t *testing.T) {
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		require.Equal(t, "/api/unstable/on_prem_runners/api_key_only", r.URL.Path)
		var body struct {
			Data struct {
				Attributes struct {
					AgentFlavor string `json:"agent_flavor"`
				} `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, flavor.PrivateActionRunner, body.Data.Attributes.AgentFlavor)
		fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
	}))
	defer srv.Close()
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"api_key": "test-key", "dd_url": srv.URL, "private_action_runner.enabled": true,
		"private_action_runner.self_enroll": true, "private_action_runner.api_key_only_enrollment": true,
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	})
	flavor.SetTestFlavor(t, flavor.PrivateActionRunner)
	hostname, _ := hostnamemock.NewMock("test-host")
	runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t)}
	_, err := runner.getRunnerConfig(context.Background())
	require.NoError(t, err)
	identity, err := enrollment.GetIdentityFromPreviousEnrollment(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.NotEmpty(t, identity.PrivateKey)
	_, err = runner.getRunnerConfig(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, posts.Load(), "a saved identity follows existing reuse behavior")
}
