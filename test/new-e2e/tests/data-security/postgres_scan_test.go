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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// sdsResultEndpoint is the intake route the sds-result event platform track posts to.
const sdsResultEndpoint = "/api/v2/sdsresult"

// postgresScanEnv is a host Agent plus a Dockerized PostgreSQL workload on the same VM.
type postgresScanEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	FakeIntake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

type postgresScanSuite struct {
	e2e.BaseSuite[postgresScanEnv]
}

func TestDataSecurityPostgresScan(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &postgresScanSuite{}, e2e.WithPulumiProvisioner(postgresScanProvisioner(), nil))
}

func (s *postgresScanSuite) TestPackagedCheckLoadsAndEmitsSDSResult() {
	// The packaged check loads and runs without error. Parse the check output
	// with the shared testcommon helper — no manual struct parsing.
	out, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"datasecurity", "--json", "--delay", "1000"}))
	require.NoError(s.T(), err, "datasecurity check failed: %s", out)
	assert.NotContains(s.T(), out, "'group' or 'others' have rights on it")
	assert.NotContains(s.T(), out, "no valid check found")

	data := check.ParseJSONOutput(s.T(), []byte(out))
	require.NotEmpty(s.T(), data, "empty check JSON: %s", out)
	assert.Equal(s.T(), 0, data[0].Runner.TotalErrors, "datasecurity check reported errors")

	// The scheduled check forwards an sds-result payload to the intake. Assert on
	// the fakeintake payload rather than the check output.
	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := fakeintake.GetRawPayloads(sdsResultEndpoint)
		require.NoError(c, err)
		// The check has a single sub task, so it emits exactly one sds-result.
		require.Len(c, payloads, 1, "expected exactly one sds-result payload at %s", sdsResultEndpoint)

		raw, err := aggregator.Inflate(payloads[0].Data, payloads[0].Encoding)
		require.NoError(c, err)
		// TODO(DATASEC-316): decode the sds-result protobuf and compare the full
		// payload
		result := string(raw)
		// The payload is an SdsResultPayload protobuf; its string fields are UTF-8,
		// so match them directly on the wire bytes.
		assert.Contains(c, result, "e2e-datasec-postgres", "sds-result payload missing task_id")
		assert.Contains(c, result, "e2e-owner-pattern", "sds-result payload missing rule_id")
		assert.Contains(c, result, "accounts", "sds-result payload missing scanned table")
		assert.NotContains(c, result, "connecting to postgres", "scan failed to connect to postgres")
	}, 2*time.Minute, 10*time.Second)
}
