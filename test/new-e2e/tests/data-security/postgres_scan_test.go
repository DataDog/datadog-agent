// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// ============================================================================
// Environment definition
// ============================================================================

// postgresScanEnv is a host Agent plus a Dockerized PostgreSQL workload on the same VM.
type postgresScanEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Docker     *components.RemoteHostDocker
}

// ============================================================================
// Test suite definition
// ============================================================================

type postgresScanSuite struct {
	e2e.BaseSuite[postgresScanEnv]
}

// TestDataSecurityPostgresScan runs the postgres scan test.
func TestDataSecurityPostgresScan(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &postgresScanSuite{}, e2e.WithPulumiProvisioner(postgresScanProvisioner(), nil))
}

// TestPackagedCheckLoadsAndEmitsSDSResult verifies that the packaged datasecurity check loads and emits an SDS result event.
func (s *postgresScanSuite) TestPackagedCheckLoadsAndEmitsSDSResult() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"datasecurity", "--json", "--delay", "1000"}))
		require.NoError(c, err, "datasecurity check failed: %s", out)
		assert.NotContains(c, out, "'group' or 'others' have rights on it")
		assert.NotContains(c, out, "no valid check found")

		root, extra := parseCheckOutput(c, []byte(out))
		assert.Equal(c, 0, root.Runner.TotalErrors, "datasecurity check reported errors")

		sdsCount := extra.Runner.EventPlatformEvents["sds-result"]
		assert.GreaterOrEqual(c, sdsCount, 1, "expected at least one sds-result event platform event")

		events := extra.Aggregator.SDSResults
		if !assert.NotEmpty(c, events, "aggregator contained no sds-result events") {
			return
		}
		raw := events[0].RawEvent
		assert.Equal(c, "sds-result", events[0].EventType)
		assert.Contains(c, raw, "e2e-datasec-postgres", "sds-result payload missing task_id")
		assert.Contains(c, raw, "e2e-owner-pattern", "sds-result payload missing rule_id")
		assert.Contains(c, raw, "accounts", "sds-result payload missing scanned table")
		assert.NotContains(c, raw, "connecting to postgres", "scan failed to connect to postgres")
		// TODO(DATASEC): assert SDS protobuf from fakeintake Client.GetRawPayloads("/api/v2/sdsresult")
	}, 2*time.Minute, 10*time.Second)
}
