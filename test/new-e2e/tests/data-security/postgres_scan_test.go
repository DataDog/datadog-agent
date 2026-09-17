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
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

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

func TestPostgresScanFixtures(t *testing.T) {
	require.NotEmpty(t, datasecurityCheckYAML())
	require.NotEmpty(t, postgresScanScenarios)
	for _, sc := range postgresScanScenarios {
		require.NotEmpty(t, sc.expected, sc.name)
		for _, want := range sc.expected {
			require.NotEmpty(t, sdsSubTaskID(want), sc.name)
		}
	}
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

	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := fakeintake.GetSDSResults()
		require.NoError(c, err)
		got := sdsResultsBySubTask(aggregatorSDSResults(payloads))
		for _, sc := range postgresScanScenarios {
			for id, want := range sdsResultsBySubTask(sc.expected) {
				assertSDSResultsEqual(c, sc.name, id, want, got[id])
			}
		}
	}, 2*time.Minute, 10*time.Second)
}

func aggregatorSDSResults(payloads []*aggregator.SDSResultPayload) []*sds.SdsResultPayload {
	out := make([]*sds.SdsResultPayload, 0, len(payloads))
	for _, p := range payloads {
		out = append(out, &p.SdsResultPayload)
	}
	return out
}

func sdsResultsBySubTask(payloads []*sds.SdsResultPayload) map[string][]*sds.SdsResultPayload {
	got := make(map[string][]*sds.SdsResultPayload)
	for _, p := range payloads {
		id := sdsSubTaskID(p)
		if id == "" {
			continue
		}
		got[id] = append(got[id], p)
	}
	return got
}

func sdsSubTaskID(p *sds.SdsResultPayload) string {
	results := p.GetScanResults()
	if len(results) == 0 {
		return ""
	}
	return results[0].GetScanMetadata().GetScanTaskMetadata().GetSubTaskId()
}

func assertSDSResultsEqual(t require.TestingT, scenario, id string, want, got []*sds.SdsResultPayload) {
	want = comparableSDSResults(want)
	got = comparableSDSResults(got)
	require.NotEmpty(t, got, "missing sds-result for scenario %q sub_task %s", scenario, id)
	for _, g := range got {
		if !containsSDSResult(want, g) {
			assert.Fail(t, "unexpected sds-result",
				"scenario %s sub_task %s\ngot:\n%s", scenario, id, protojson.Format(g))
		}
	}
	for _, w := range want {
		if !containsSDSResult(got, w) {
			assert.Fail(t, "missing sds-result",
				"scenario %s sub_task %s\nwant:\n%s", scenario, id, protojson.Format(w))
		}
	}
}

func containsSDSResult(haystack []*sds.SdsResultPayload, needle *sds.SdsResultPayload) bool {
	for _, p := range haystack {
		if proto.Equal(p, needle) {
			return true
		}
	}
	return false
}

func comparableSDSResults(in []*sds.SdsResultPayload) []*sds.SdsResultPayload {
	out := make([]*sds.SdsResultPayload, len(in))
	for i, p := range in {
		out[i] = comparableSDSResult(p)
	}
	return out
}

// comparableSDSResult is the single place that strips fields we do not assert
// (timestamp, failure_reason) before proto.Equal.
func comparableSDSResult(p *sds.SdsResultPayload) *sds.SdsResultPayload {
	out := proto.Clone(p).(*sds.SdsResultPayload)
	out.Timestamp = 0
	for _, result := range out.GetScanResults() {
		if meta := result.GetScanMetadata().GetScanTaskMetadata(); meta != nil {
			meta.FailureReason = nil
		}
	}
	return out
}
