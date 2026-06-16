// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

func TestBuildIssue_SchemaViolationProducesMediumSeverity(t *testing.T) {
	ctx := map[string]string{
		contextKeyConfigPath: "/etc/datadog-agent/datadog.yaml",
		contextKeyErrorCount: "3",
	}
	ctx[contextErrorKey(0)] = "at '/agent_ipc/port': got string, want integer"
	ctx[contextErrorKey(1)] = "at '/tags': got object, want array"
	issue, err := InvalidConfigIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Empty(t, issue.GetId(), "Id is set by the runner (ReportIssue), not by the template")
	assert.Equal(t, IssueID, issue.GetIssueName())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Contains(t, issue.GetTitle(), "3 schema violations")
	assert.Equal(t, float64(3),
		issue.GetExtra().GetFields()[contextKeyErrorCount].GetNumberValue())
	assert.Contains(t, issue.GetDescription(), "agent_ipc/port")
	assert.Contains(t, issue.GetDescription(), "/tags")
	assert.Contains(t, issue.GetDescription(), "; ", "description must use a visible delimiter between violations so the UI renders them legibly")

	errorsStruct := issue.GetExtra().GetFields()[contextKeyErrors].GetStructValue()
	require.NotNil(t, errorsStruct, "extra.errors must be a struct with one entry per violation")
	assert.Len(t, errorsStruct.GetFields(), 2, "each violation must get its own key")
	assert.Equal(t, "got string, want integer", errorsStruct.GetFields()["/agent_ipc/port"].GetListValue().GetValues()[0].GetStringValue())
	assert.Equal(t, "got object, want array", errorsStruct.GetFields()["/tags"].GetListValue().GetValues()[0].GetStringValue())
}

// A vanilla mock has only defaults, which round-trip through YAML cleanly and
// pass the schema. Confirms Run() is a no-op on a healthy config.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := newChecker(config.NewMock(t)).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// Inject a string into an integer-typed field. Confirms the validator surfaces
// the violation and the checker wraps it into an IssueReport.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("agent_ipc.port", "not-a-number")

	reports, err := newChecker(cfg).Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, IssueID, reports[0].IssueName)
	assert.Equal(t, IssueID, reports[0].IssueID)
	assert.Contains(t, reports[0].Context[contextErrorKey(0)], "agent_ipc/port")
}

// Integration test: checker.Run() → BuildIssue() → HTTP POST to in-process
// fakeintake → GetAgentHealth(). Requires the embedded schema (Bazel only).
func TestCheck_ExtraErrorsSurviveHTTPRoundtrip(t *testing.T) {
	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(fakeintakeserver.WithReadyChannel(ready))
	fi.Start()
	require.True(t, <-ready, "fakeintake server failed to start")
	t.Cleanup(func() { _ = fi.Stop() })

	fic := fakeintakeclient.NewClient(fi.URL(), fakeintakeclient.WithoutStrictFakeintakeIDCheck())

	tests := []struct {
		name         string
		buildContext func(t *testing.T) map[string]string
		// wantErrors maps each expected config path to the substrings that must
		// appear in its error messages (one substring per message, in order).
		wantErrors map[string][]string
	}{
		{
			name: "1 misconfiguration",
			buildContext: func(t *testing.T) map[string]string {
				cfg := config.NewMock(t)
				cfg.SetInTest("agent_ipc.port", "not-a-number")
				reports, err := newChecker(cfg).Run()
				require.NoError(t, err)
				require.Len(t, reports, 1)
				return reports[0].Context
			},
			wantErrors: map[string][]string{
				"/agent_ipc/port": {"got string, want integer"},
			},
		},
		{
			name: "2 misconfigurations on different keys",
			buildContext: func(t *testing.T) map[string]string {
				cfg := config.NewMock(t)
				cfg.SetInTest("agent_ipc.port", "not-a-number")
				cfg.SetInTest("agent_ipc.config_refresh_interval", "also-not-a-number")
				reports, err := newChecker(cfg).Run()
				require.NoError(t, err)
				require.Len(t, reports, 1)
				return reports[0].Context
			},
			wantErrors: map[string][]string{
				"/agent_ipc/port":                    {"got string, want integer"},
				"/agent_ipc/config_refresh_interval": {"got string, want integer"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, fic.FlushServerAndResetAggregators())

			issue, err := InvalidConfigIssue{}.BuildIssue(tt.buildContext(t))
			require.NoError(t, err)

			rep := &healthplatform.HealthReport{
				EventType: "agent-health-issues",
				Issues:    map[string]*healthplatform.Issue{IssueID: issue},
			}
			body, err := json.Marshal(rep)
			require.NoError(t, err)

			resp, err := http.Post(fi.URL()+"/api/v2/agenthealth", "application/json", bytes.NewReader(body))
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			payloads, err := fic.GetAgentHealth()
			require.NoError(t, err)
			require.Len(t, payloads, 1)

			receivedIssue := payloads[0].Issues[IssueID]
			require.NotNil(t, receivedIssue)

			errorsStruct := receivedIssue.GetExtra().GetFields()[contextKeyErrors].GetStructValue()
			require.NotNil(t, errorsStruct, "extra.errors must reach the intake as a path-keyed struct")
			assert.Len(t, errorsStruct.GetFields(), len(tt.wantErrors))

			for path, wantMsgs := range tt.wantErrors {
				vals := errorsStruct.GetFields()[path].GetListValue().GetValues()
				require.Lenf(t, vals, len(wantMsgs), "path %s", path)
				for i, want := range wantMsgs {
					assert.Equalf(t, want, vals[i].GetStringValue(), "path %s message %d", path, i)
				}
			}
		})
	}
}
