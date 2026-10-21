// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
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

//go:embed fixtures/rc/scenarios/*.yaml
var rcScenarioFS embed.FS

var rcIDPlaceholder = regexp.MustCompile(`\$\{(?:TASK|SUB_TASK)_ID(?:_[A-Z0-9_]+)?\}`)

// rcScenario is self-contained: the RC payload, the apply state the agent must report, and the
// SDS results that must follow.
type rcScenario struct {
	Name       string           `yaml:"name"`
	RC         map[string]any   `yaml:"rc"`
	ApplyState string           `yaml:"apply_state"`
	ApplyError string           `yaml:"apply_error"` // regexp, only for apply_state: error
	Expected   []map[string]any `yaml:"expected"`
}

var rcApplyStates = map[string]state.ApplyState{
	"acknowledged": state.ApplyStateAcknowledged,
	"error":        state.ApplyStateError,
}

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

// TestRCScenarios pushes each scenario through Remote Config and checks the apply state the
// agent reports and the SDS results that reach fakeintake.
func (s *postgresRCSuite) TestRCScenarios() {
	for _, sc := range loadRCScenarios(s.T()) {
		s.Run(sc.Name, func() {
			payload, err := json.Marshal(sc.RC)
			require.NoError(s.T(), err)
			applyState, ok := rcApplyStates[sc.ApplyState]
			require.True(s.T(), ok, "unknown apply_state %q", sc.ApplyState)

			id := fmt.Sprintf("e2e-datasec-%s-%s", sc.Name, uuid.NewString())
			fi := s.Env().FakeIntake.Client()
			require.NoError(s.T(), fi.RCAddConfig("", state.ProductDataSecurityDBScanTasks, id, "scan-task", payload))

			pattern := fmt.Sprintf(`config_states:\{id:%q[^}]*apply_state:%d`, id, applyState)
			if sc.ApplyError != "" {
				pattern += `[^}]*apply_error:".*` + sc.ApplyError
			}
			s.EventuallyWithT(func(c *assert.CollectT) {
				assert.Regexp(c, pattern, s.Env().Agent.Client.RemoteConfig())
			}, time.Minute, 5*time.Second)

			if len(sc.Expected) == 0 {
				return // rejected task: nothing is scanned
			}
			s.EventuallyWithT(func(c *assert.CollectT) {
				payloads, err := fi.GetSDSResults()
				require.NoError(c, err)
				got := sdsResultsBySubTask(aggregatorSDSResults(payloads))
				for subTaskID, want := range sdsResultsBySubTask(sc.expectedResults(s.T())) {
					assertSDSResultsEqual(c, sc.Name, subTaskID, want, got[subTaskID])
				}
			}, 2*time.Minute, 10*time.Second)
		})
	}
}

func (sc rcScenario) expectedResults(t *testing.T) []*sds.SdsResultPayload {
	t.Helper()
	out := make([]*sds.SdsResultPayload, 0, len(sc.Expected))
	for _, raw := range sc.Expected {
		out = append(out, mustSDSResult(sc.Name, raw))
	}
	return out
}

func loadRCScenarios(t *testing.T) []rcScenario {
	t.Helper()
	entries, err := rcScenarioFS.ReadDir("fixtures/rc/scenarios")
	require.NoError(t, err)
	scenarios := make([]rcScenario, 0, len(entries))
	for _, entry := range entries {
		raw, err := rcScenarioFS.ReadFile("fixtures/rc/scenarios/" + entry.Name())
		require.NoError(t, err)
		ids := make(map[string]string)
		raw = rcIDPlaceholder.ReplaceAllFunc(raw, func(placeholder []byte) []byte {
			key := string(placeholder)
			if _, found := ids[key]; !found {
				ids[key] = uuid.NewString()
			}
			return []byte(ids[key])
		})
		require.NotContains(t, string(raw), "${", "%s: unresolved ID placeholder", entry.Name())
		var sc rcScenario
		require.NoError(t, yaml.Unmarshal(raw, &sc), entry.Name())
		scenarios = append(scenarios, sc)
	}
	return scenarios
}
