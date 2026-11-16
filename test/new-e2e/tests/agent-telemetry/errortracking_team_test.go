// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetry

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed testdata/errortracking-team.yaml
var errorTrackingTeamConfig string

//go:embed testdata/errortracking-team-logs.yaml
var errorTrackingTeamLogsConfig string

// teamFault is an error induced in code owned by a known team. The expected
// team is the CODEOWNERS owner of sourceFile on this checkout, so a change of
// owner for these files needs the same change here.
type teamFault struct {
	name string
	// sourceFile is the repository-relative file the error is logged from; it
	// identifies the fault's records in the stack trace.
	sourceFile string
	team       string
}

var teamFaults = []teamFault{
	// The logs agent TCP listener cannot bind a port that is already in use.
	{name: "logs", sourceFile: "pkg/logs/launchers/listener/tcp.go", team: "agent-log-pipelines"},
	// A check that raises makes the collector worker log "Error running check".
	{name: "collector", sourceFile: "pkg/collector/worker/check_logger.go", team: "agent-runtimes"},
	// A malformed request to the trace-agent receiver.
	{name: "trace", sourceFile: "pkg/trace/api/api.go", team: "agent-apm"},
}

type errorTrackingTeamSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestErrorTrackingTeamSuite provisions one host with three faults owned by
// three different teams, so a constant team tag cannot satisfy the test.
func TestErrorTrackingTeamSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &errorTrackingTeamSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingTeamConfig),
						agentparams.WithIntegration("errortracking_team_logs.d", errorTrackingTeamLogsConfig),
						agentparams.WithIntegration("error_check.d", errorCheckConfig),
						agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
					),
				),
			),
		),
	)
}

// teamTagValues returns the values of every team: tag in a CSV tags string.
func teamTagValues(tags string) []string {
	var teams []string
	for _, tag := range strings.Split(tags, ",") {
		if team, ok := strings.CutPrefix(tag, "team:"); ok {
			teams = append(teams, team)
		}
	}
	return teams
}

// TestTeamTagFollowsOwner checks that each fault's error logs carry the team
// that owns the failing code, and that the faults do not share a team.
func (s *errorTrackingTeamSuite) TestTeamTagFollowsOwner() {
	t := s.T()
	env := s.Env()
	require.NoError(t, env.FakeIntake.Client().FlushServerAndResetAggregators())
	// The logs listener fails once at startup, so restart the agent after the flush above.
	env.RemoteHost.MustExecute("sudo systemctl restart datadog-agent")

	var byFault map[string][]*aggregator.AgentTelemetryLog
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// The receiver error only fires on demand; the others recur or fired at startup.
		_, err := env.RemoteHost.Execute(
			`curl -s -o /dev/null -X POST http://127.0.0.1:8126/v0.4/traces -H "X-Datadog-Trace-Count: notanumber"`)
		assert.NoError(c, err)

		logs, err := env.FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)
		byFault = map[string][]*aggregator.AgentTelemetryLog{}
		for _, l := range logs {
			for _, f := range teamFaults {
				if strings.Contains(l.StackTrace, f.sourceFile) {
					byFault[f.name] = append(byFault[f.name], l)
				}
			}
		}
		for _, f := range teamFaults {
			assert.NotEmpty(c, byFault[f.name], "no error log from %s (%s) received yet", f.name, f.sourceFile)
		}
	}, 3*time.Minute, 5*time.Second, "timed out waiting for an error log from every fault")

	seen := map[string]bool{}
	for _, f := range teamFaults {
		for _, l := range byFault[f.name] {
			assert.Equal(t, []string{f.team}, teamTagValues(l.Tags),
				"%s fault must carry exactly team:%s; tags: %q", f.name, f.team, l.Tags)
		}
		seen[f.team] = true
	}
	assert.Len(t, seen, len(teamFaults), "faults must be owned by distinct teams")
}
