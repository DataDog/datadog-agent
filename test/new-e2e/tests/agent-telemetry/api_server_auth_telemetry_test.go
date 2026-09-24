// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	agentCmdPort     = 5001
	traceDebugPort   = 5012 // default of apm_config.debug.port
	agentAPIFlavor   = "agent"
	traceAgentFlavor = "trace-agent"
	authTokenPath    = "/etc/datadog-agent/auth_token"

	// apiServerRequestDurationMetricName is how the metric created by
	// comp/api/api/apiimpl/observability (api_server__request_duration_seconds)
	// appears in the agent telemetry payload.
	apiServerRequestDurationMetricName = "api_server.request_duration_seconds"
)

// agentTelemetryConfig turns on agent telemetry so the payload endpoint
// (GET /agent/metadata/agent-telemetry) is active. apm_config.enabled and
// remote_agent.registry.enabled both default to true, so the trace agent
// runs and registers with the core agent's remote agent registry without
// any extra configuration.
const agentTelemetryConfig = `
agent_telemetry:
  enabled: true
`

type apiServerAuthTelemetrySuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAPIServerAuthTelemetry verifies that the api_server request telemetry
// carrying the IPC authentication mode (mTLS vs token) is emitted not only by
// the core agent but also by remote agents, whose series reach the core agent
// through the remote agent registry telemetry provider.
func TestAPIServerAuthTelemetry(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &apiServerAuthTelemetrySuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(agentTelemetryConfig),
					),
				),
			),
		),
	)
}

// authToken returns the IPC auth token of the installed agent.
func (s *apiServerAuthTelemetrySuite) authToken(c *assert.CollectT) string {
	return strings.TrimSpace(s.Env().RemoteHost.MustExecuteOn(c, "sudo cat "+authTokenPath))
}

// getAgentTelemetryPayload fetches the agent telemetry payload from the core
// agent, over a plain token-authenticated request (no client certificate), and
// returns the decoded JSON. The request itself goes through the core agent's
// instrumented CMD API, so it records an api_server series tagged auth=token.
func (s *apiServerAuthTelemetrySuite) getAgentTelemetryPayload(c *assert.CollectT) any {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://localhost:%d/agent/metadata/agent-telemetry", agentCmdPort), nil)
	require.NoError(c, err, "failed to build request")
	req.Header.Set("Authorization", "Bearer "+s.authToken(c))

	resp, err := s.Env().RemoteHost.NewHTTPClient().Do(req)
	require.NoError(c, err, "failed to request the agent telemetry payload")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(c, err, "failed to read the agent telemetry payload")
	require.Equal(c, http.StatusOK, resp.StatusCode, "agent telemetry payload request failed: %s", body)

	var payload any
	require.NoError(c, json.Unmarshal(body, &payload), "failed to parse the agent telemetry payload: %s", body)
	return payload
}

// requestTraceAgentDebugConfig sends a token-authenticated request (no client
// certificate) to the trace agent's instrumented debug /config endpoint.
func (s *apiServerAuthTelemetrySuite) requestTraceAgentDebugConfig(c *assert.CollectT) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/config", traceDebugPort), nil)
	require.NoError(c, err, "failed to build request")
	req.Header.Set("Authorization", "Bearer "+s.authToken(c))

	resp, err := s.Env().RemoteHost.NewHTTPClient().Do(req)
	require.NoError(c, err, "failed to request the trace agent debug config endpoint")
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	require.Equal(c, http.StatusOK, resp.StatusCode, "trace agent debug config request failed")
}

// collectAPIServerSeries walks the agent telemetry payload JSON and returns
// the tag sets of every api_server.request_duration_seconds series it
// contains. The payload nests differently for single vs batched requests, so
// a recursive walk is simpler than modeling every wrapper.
func collectAPIServerSeries(v any) []map[string]string {
	var out []map[string]string

	switch node := v.(type) {
	case map[string]any:
		for key, child := range node {
			if key == apiServerRequestDurationMetricName {
				if tags := seriesTags(child); tags != nil {
					out = append(out, tags)
				}
				continue
			}
			out = append(out, collectAPIServerSeries(child)...)
		}
	case []any:
		for _, child := range node {
			out = append(out, collectAPIServerSeries(child)...)
		}
	}
	return out
}

func seriesTags(v any) map[string]string {
	series, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	rawTags, ok := series["tags"].(map[string]any)
	if !ok {
		return nil
	}
	tags := make(map[string]string, len(rawTags))
	for name, value := range rawTags {
		if s, ok := value.(string); ok {
			tags[name] = s
		}
	}
	return tags
}

func hasSeriesWithTags(series []map[string]string, want map[string]string) bool {
	for _, tags := range series {
		matches := true
		for name, value := range want {
			if tags[name] != value {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

// TestCoreAgentAuthTag checks that a token-authenticated request to the core
// agent's CMD API is reported by the agent telemetry with auth tag "token".
func (s *apiServerAuthTelemetrySuite) TestCoreAgentAuthTag() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		series := collectAPIServerSeries(s.getAgentTelemetryPayload(c))
		assert.True(c, hasSeriesWithTags(series, map[string]string{
			"emitter": agentAPIFlavor,
			"auth":    "token",
			"path":    "/metadata/agent-telemetry",
		}), "no api_server series tagged emitter=%s,auth=token for the payload request itself; got %v", agentAPIFlavor, series)
	}, 3*time.Minute, 10*time.Second)
}

// TestRemoteAgentAuthTag checks the full remote agent telemetry chain: a
// token-authenticated request served by the trace agent is reported by the
// trace agent's api_server telemetry, forwarded to the core agent through the
// remote agent registry telemetry provider, and exposed in the core agent's
// agent telemetry payload tagged with the trace agent's emitter.
func (s *apiServerAuthTelemetrySuite) TestRemoteAgentAuthTag() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.requestTraceAgentDebugConfig(c)
		series := collectAPIServerSeries(s.getAgentTelemetryPayload(c))
		assert.True(c, hasSeriesWithTags(series, map[string]string{
			"emitter": traceAgentFlavor,
			"auth":    "token",
		}), "no api_server series tagged emitter=%s,auth=token forwarded by the trace agent; got %v", traceAgentFlavor, series)
	}, 5*time.Minute, 15*time.Second)
}
