// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

// The scan task carries no connection: the provider resolves it from the postgres integration
// whose host matches the task's database_host_name.
const rcPostgresYAML = `
init_config:
instances:
  - host: localhost
    port: 5432
    dbname: labdb
    username: postgres
    password: postgres_admin_pw
`

//go:embed fixtures/rc/scan_task.json
var rcScanTask []byte

//go:embed fixtures/rc/expected.yaml
var rcExpectedYAML []byte

type postgresRCSuite struct {
	e2e.BaseSuite[postgresScanEnv]
}

func TestDataSecurityPostgresScanRC(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &postgresRCSuite{}, e2e.WithPulumiProvisioner(
		postgresScanProvisioner(agentparams.WithIntegration("postgres.d", rcPostgresYAML)), nil))
}

// SetupSuite drops RC configs left by a previous run (reused stack).
func (s *postgresRCSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	fi := s.Env().FakeIntake.Client()
	configs, err := fi.RCListConfigs()
	require.NoError(s.T(), err)
	for _, c := range configs {
		require.NoError(s.T(), fi.RCDeleteConfig(fmt.Sprintf("%s/%s/%s/%s", c.OrgID, c.Product, c.ConfigID, c.ConfigName)))
	}
}

// TestRCScanTaskEmitsSDSResult covers the golden path: an RC scan task is scheduled by the
// data-security provider, scanned, and its SDS result reaches fakeintake.
func (s *postgresRCSuite) TestRCScanTaskEmitsSDSResult() {
	expected := rcExpectedResults(s.T())

	id := fmt.Sprintf("e2e-datasec-%d", time.Now().UnixNano())
	fi := s.Env().FakeIntake.Client()
	require.NoError(s.T(), fi.RCAddConfig("", state.ProductDataSecurityDBScanTasks, id, "scan-task", rcScanTask))

	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := fi.GetSDSResults()
		require.NoError(c, err)
		got := sdsResultsBySubTask(aggregatorSDSResults(payloads))
		for subTaskID, want := range sdsResultsBySubTask(expected) {
			assertSDSResultsEqual(c, "rc", subTaskID, want, got[subTaskID])
		}
	}, 2*time.Minute, 10*time.Second)
}

func rcExpectedResults(t *testing.T) []*sds.SdsResultPayload {
	t.Helper()
	var doc struct {
		Expected []map[string]any `yaml:"expected"`
	}
	require.NoError(t, yaml.Unmarshal(rcExpectedYAML, &doc))
	out := make([]*sds.SdsResultPayload, 0, len(doc.Expected))
	for _, raw := range doc.Expected {
		out = append(out, mustSDSResult("rc", raw))
	}
	return out
}
