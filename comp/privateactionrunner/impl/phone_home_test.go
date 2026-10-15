// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
)

func TestPhoneHomeEnrollmentOutcome(t *testing.T) {
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "true")
	for _, tc := range []struct {
		name               string
		status             int
		persistenceFailure bool
	}{
		{"success", 200, false}, {"rejected", 403, false}, {"ambiguous_5xx", 503, false}, {"response_lost", 0, false}, {"persistence_failed", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int32
			identityPath := filepath.Join(t.TempDir(), "identity.json")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				posts.Add(1)
				if tc.status == 0 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close() // backend may have committed; response never arrived
					return
				}
				if tc.persistenceFailure {
					require.NoError(t, os.Mkdir(identityPath, 0700))
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
			}))
			defer srv.Close()
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"api_key": "test-key", "dd_url": srv.URL,
				"private_action_runner.enabled":                 true,
				"private_action_runner.split_enabled":           true,
				"private_action_runner.self_enroll":             true,
				"private_action_runner.api_key_only_enrollment": true,
				"private_action_runner.identity_file_path":      identityPath,
			})
			a, err := enrollment.ReservePhoneHomeAttempt(cfg, "test-host", time.Now(), nil)
			require.NoError(t, err)
			hostname, _ := hostnamemock.NewMock("test-host")
			runner := &PrivateActionRunner{coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t)}
			resolved, err := runner.getRunnerConfig(context.Background())
			if tc.status == 200 && !tc.persistenceFailure {
				require.NoError(t, err)
				snapshot, err := executor.ControlPlaneConfig(cfg, resolved)
				require.NoError(t, err)
				require.True(t, snapshot.SplitMode)
				// New executor (e.g. after idle exit) reuses persisted identity.
				_, err = runner.getRunnerConfig(context.Background())
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				outcome, err := enrollment.ReadPhoneHomeOutcome(cfg, a)
				require.NoError(t, err)
				require.NotNil(t, outcome)
				expected := map[string]string{"rejected": "forbidden_unknown", "ambiguous_5xx": "response_ambiguous", "response_lost": "transport_ambiguous", "persistence_failed": "persistence_pending"}
				require.Equal(t, expected[tc.name], outcome.Category)
				// A rejected/pending POC must never serve a disabled snapshot,
				// and a restarted executor must not submit another mutation.
				_, resolved, err = runner.configureExecutor(context.Background(), context.Background())
				require.Error(t, err)
				require.Nil(t, resolved)
				require.Nil(t, runner.executorServer)
			}
			require.EqualValues(t, 1, posts.Load())
		})
	}
}
