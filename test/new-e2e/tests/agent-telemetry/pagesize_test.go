// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetry

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// pageSizeAgentConfig replaces the default agent telemetry profiles with one that reports
// runtime.running every 30s, so a metrics payload reaches the intake within a minute of the
// Agent starting rather than on the default profiles' 15-minute period.
const pageSizeAgentConfig = `agent_telemetry:
  enabled: true
  profiles:
    - name: page-size
      metric:
        metrics:
          - name: runtime.running
      schedule:
        start_after: 5
        period: 30
`

// pageSizeSuite checks that agent telemetry reports the page size of the kernel the installed
// Agent runs on (agent_metadata.page_size, set in comp/core/agenttelemetry/impl/sender.go).
//
// 4KiB and 16KiB pages are covered by TestPageSize in comp/core/agenttelemetry/impl, which runs
// on every CI runner OS. This suite exists for the kernels no CI runner has, and to check the
// payloads that reach the intake from a deployed Agent.
type pageSizeSuite struct {
	e2e.BaseSuite[environments.Host]

	// pageSize is the page size of the provisioned kernel. It is hardcoded per entry point rather
	// than read from the host, so that an image whose kernel changes page size fails the suite
	// instead of quietly covering a different page size.
	pageSize int
}

type pageSizeX64Suite struct{ pageSizeSuite }

// pageSizeARM64Suite installs the arm64 RPM, which new-e2e-agent-telemetry doesn't pull, so it
// runs in its own job, new-e2e-agent-telemetry-arm64.
type pageSizeARM64Suite struct{ pageSizeSuite }

func pageSizeProvisioner(osDesc e2eos.Descriptor) e2e.SuiteOption {
	return e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(osDesc)),
				ec2.WithAgentOptions(agentparams.WithAgentConfig(pageSizeAgentConfig)),
			),
		),
	)
}

func TestPageSizeLinuxX64Suite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &pageSizeX64Suite{pageSizeSuite{pageSize: 4096}}, pageSizeProvisioner(e2eos.UbuntuDefault))
}

// TestPageSizeLinuxARM64Suite runs on CentOS 7, whose arm64 kernel is built with 64KiB pages.
func TestPageSizeLinuxARM64Suite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &pageSizeARM64Suite{pageSizeSuite{pageSize: 65536}},
		pageSizeProvisioner(e2eos.CentOS7.WithArch(e2eos.ARM64Arch)))
}

// TestHostPageSize guards the other tests: if the image's kernel stopped having the expected page
// size, they would keep passing while no longer covering it.
func (s *pageSizeSuite) TestHostPageSize() {
	out := s.Env().RemoteHost.MustExecute("getconf PAGESIZE")
	assert.Equal(s.T(), strconv.Itoa(s.pageSize), strings.TrimSpace(out))
}

// TestDiagnoseReportsPageSize checks the payload the running Agent builds.
func (s *pageSizeSuite) TestDiagnoseReportsPageSize() {
	out := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-telemetry"}))

	// The command output can contain log lines around the JSON payload
	start, end := strings.Index(out, "{"), strings.LastIndex(out, "}")
	require.True(s.T(), start >= 0 && end > start, "no JSON payload in output: %s", out)
	sizes, err := pageSizes([]byte(out[start : end+1]))
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), sizes, "no agent-metrics message in output: %s", out)
	for _, size := range sizes {
		assert.Equal(s.T(), s.pageSize, size)
	}
}

// TestPayloadsReportPageSize checks the payloads that reach the intake.
func (s *pageSizeSuite) TestPayloadsReportPageSize() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetRawPayloads("/api/v2/apmtelemetry")
		require.NoError(c, err)

		var sizes []int
		for _, p := range payloads {
			body, err := aggregator.Inflate(p.Data, p.Encoding)
			require.NoError(c, err)
			// The endpoint is shared with other telemetry, not all of it JSON
			if got, err := pageSizes(body); err == nil {
				sizes = append(sizes, got...)
			}
		}
		require.NotEmpty(c, sizes, "no agent-metrics payload received yet")
		for _, size := range sizes {
			assert.Equal(c, s.pageSize, size)
		}
	}, 3*time.Minute, 10*time.Second)
}

// telemetryMessage is the envelope of an agent telemetry payload, and of each message of a
// message-batch payload.
type telemetryMessage struct {
	RequestType string          `json:"request_type"`
	Payload     json.RawMessage `json:"payload"`
}

// pageSizes returns agent_metadata.page_size from every agent-metrics message of an agent
// telemetry payload, or 0 for a message that lacks it. Other request types are skipped.
func pageSizes(body []byte) ([]int, error) {
	var top telemetryMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	messages := []telemetryMessage{top}
	if top.RequestType == "message-batch" {
		messages = nil
		if err := json.Unmarshal(top.Payload, &messages); err != nil {
			return nil, err
		}
	}

	var sizes []int
	for _, m := range messages {
		if m.RequestType != "agent-metrics" {
			continue
		}
		var payload struct {
			Metrics struct {
				AgentMetadata struct {
					PageSize int `json:"page_size"`
				} `json:"agent_metadata"`
			} `json:"metrics"`
		}
		if err := json.Unmarshal(m.Payload, &payload); err != nil {
			return nil, err
		}
		sizes = append(sizes, payload.Metrics.AgentMetadata.PageSize)
	}
	return sizes, nil
}
