// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func TestPrintCheckExecutionStatus_ExactMatch(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "my_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}

	instanceIDs := []string{"my_check:abc123"}
	stats := types.CLCRunnersStats{
		"my_check:abc123": {
			AverageExecutionTime: 100,
			TotalRuns:            50,
			MetricSamples:        10,
			LastExecFailed:       false,
		},
	}

	printCheckExecutionStatus(&buf, config, stats, "", instanceIDs)
	output := buf.String()

	assert.Contains(t, output, "[OK]")
	assert.Contains(t, output, "Total Runs: 50")
	assert.Contains(t, output, "Metric Samples: Last Run: 10")
	assert.Contains(t, output, "100ms")
}

func TestPrintCheckExecutionStatus_MultipleInstances(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "http_check",
		InitConfig: integration.Data("{}"),
		Instances: []integration.Data{
			integration.Data(`{"name": "instance_a", "url": "http://a.com"}`),
			integration.Data(`{"name": "instance_b", "url": "http://b.com"}`),
		},
	}

	instanceIDs := []string{
		"http_check:instance_a:abc123",
		"http_check:instance_b:def456",
	}
	stats := types.CLCRunnersStats{
		"http_check:instance_a:abc123": {
			AverageExecutionTime: 100,
			TotalRuns:            10,
		},
		"http_check:instance_b:def456": {
			AverageExecutionTime: 200,
			TotalRuns:            20,
		},
	}

	printCheckExecutionStatus(&buf, config, stats, "", instanceIDs)
	output := buf.String()

	assert.Contains(t, output, "Total Runs: 10")
	assert.Contains(t, output, "Total Runs: 20")

	lines := strings.Split(output, "\n")
	instanceCount := 0
	for _, line := range lines {
		if strings.Contains(line, "Instance ID:") {
			instanceCount++
		}
	}
	assert.Equal(t, 2, instanceCount)
}

func TestPrintCheckExecutionStatus_EmptyStats(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "my_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}

	printCheckExecutionStatus(&buf, config, types.CLCRunnersStats{}, "", []string{"my_check:abc123"})
	assert.Empty(t, buf.String())
}

func TestPrintCheckExecutionStatus_NoMatch(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "my_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}

	stats := types.CLCRunnersStats{
		"other_check:abc123": {
			TotalRuns: 100,
		},
	}

	printCheckExecutionStatus(&buf, config, stats, "", []string{"my_check:xyz789"})
	assert.Empty(t, buf.String())
}

func TestPrintCheckExecutionStatus_FilterByCheckName(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "my_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}

	instanceIDs := []string{"my_check:abc123"}
	stats := types.CLCRunnersStats{
		"my_check:abc123": {
			TotalRuns: 100,
		},
	}

	// Filter for a different check name — should print nothing
	printCheckExecutionStatus(&buf, config, stats, "other_check", instanceIDs)
	assert.Empty(t, buf.String())

	// Filter for matching check name — should print stats
	buf.Reset()
	printCheckExecutionStatus(&buf, config, stats, "my_check", instanceIDs)
	assert.Contains(t, buf.String(), "Total Runs: 100")
}

func TestPrintCheckExecutionStatus_ErrorFields(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "my_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}

	instanceIDs := []string{"my_check:abc123"}
	stats := types.CLCRunnersStats{
		"my_check:abc123": {
			LastExecFailed:    true,
			LastError:         "timeout after 30s",
			TotalRuns:         100,
			TotalErrors:       5,
			LastSuccessDate:   1775563775,
			LastExecutionDate: 1775563800000,
		},
	}

	printCheckExecutionStatus(&buf, config, stats, "", instanceIDs)
	output := buf.String()

	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "timeout after 30s")
	assert.Contains(t, output, "Last Execution Date")
	assert.Contains(t, output, "Last Successful Execution Date")
}

func TestPrintCheckExecutionStatus_NilInstanceIDs(t *testing.T) {
	var buf bytes.Buffer

	config := integration.Config{
		Name:       "my_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}

	stats := types.CLCRunnersStats{
		"my_check:abc123": {
			TotalRuns: 100,
		},
	}

	// Nil instanceIDs — should print nothing (no IDs to match)
	printCheckExecutionStatus(&buf, config, stats, "", nil)
	assert.Empty(t, buf.String())
}

func TestRenderClusterChecks(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	utilization := 0.25
	state := types.StateResponse{
		Dispatching: &types.DispatchingState{
			AdvancedDispatching:   true,
			RebalancePeriod:       "10m0s",
			NodeExpirationSeconds: 30,
			RunnerGroups:          []types.RunnerGroupState{{Name: "kube", Checks: []string{"kubernetes_state_core"}}, {Name: "http", Checks: []string{"http_check"}}},
		},
		Dangling:          []integration.Config{{Name: "http_check", ClusterCheck: true}},
		UnassignedReasons: map[string]string{"http_check": `runner group "http" has no live runner`},
		Nodes: []types.StateNodeResponse{
			{
				Name: "runner-1", NodeType: types.NodeTypeCLCRunner, Workers: 4, Utilization: &utilization, LastHeartbeat: now.Unix() - 3,
				Configs: []types.ConfigWithInstanceIDs{{Config: integration.Config{Name: "redis"}}},
				Stats:   types.CLCRunnersStats{"redis:1": {IsClusterCheck: true, LastExecFailed: true}},
			},
			{Name: "kube-1", NodeType: types.NodeTypeCLCRunner, Group: "kube", LastHeartbeat: now.Unix() - 27},
			{Name: "kube-2", NodeType: types.NodeTypeCLCRunner, Group: "kube", LastHeartbeat: now.Unix() - 20},
			{Name: "rogue-1", NodeType: types.NodeTypeCLCRunner, Group: "foo", LastHeartbeat: now.Unix() - 1},
		},
	}

	var b bytes.Buffer
	renderClusterChecks(&b, state, ClusterChecksOptions{}, now)
	out := b.String()

	assert.Contains(t, out, "Advanced dispatching: enabled, rebalanced every 10m0s")
	assert.Contains(t, out, "Excluded checks: none")
	assert.Regexp(t, `kube\s+kubernetes_state_core\s+2\n`, out)
	assert.Regexp(t, `http\s+http_check\s+0\n`, out)
	assert.Regexp(t, `general\s+all other checks\s+1\n`, out)
	assert.Contains(t, out, `rogue-1 declares unknown runner group "foo"`)
	assert.Contains(t, out, "runner-1: 1 check instance(s) failed their last run")
	assert.Contains(t, out, "kube-1: last seen 27s ago, expires after 30s without a heartbeat")
	// A busy runner only sends its backup heartbeat 5s before expiring.
	assert.NotContains(t, out, "kube-2: last seen")
	assert.Contains(t, out, "=== 1 unassigned configuration(s) ===\nCheck   ")
	assert.Regexp(t, `http_check\s+0\s+runner group "http" has no live runner`, out)
	assert.Contains(t, out, "=== 4 agent(s) reporting ===\nName   ")
	assert.Regexp(t, `runner-1\s+clc-runner\s+general\s+4\s+25.0%\s+1\s+3s ago`, out)
	assert.Regexp(t, `kube-2\s+clc-runner\s+kube\s+-\s+-\s+0\s+20s ago`, out)
	assert.Less(t, strings.Index(out, "=== Runner group general ==="), strings.Index(out, "Checks on runner-1"))
}

func TestRenderClusterChecksAdvancedDispatchingDisabled(t *testing.T) {
	var b bytes.Buffer
	renderClusterChecks(&b, types.StateResponse{
		Dispatching: &types.DispatchingState{AdvancedDispatchingReason: "node agents are reporting", ExcludedChecks: []string{"a", "b"}},
	}, ClusterChecksOptions{}, time.Now())
	out := b.String()

	assert.Contains(t, out, "Advanced dispatching: disabled (node agents are reporting)")
	assert.Contains(t, out, "Excluded checks: a, b")
	assert.NotContains(t, out, "Runner groups")
	assert.Contains(t, out, "Zero agent reporting")
}

func TestRenderClusterChecksInstances(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	state := types.StateResponse{
		Nodes: []types.StateNodeResponse{{
			Name: "runner-1",
			Configs: []types.ConfigWithInstanceIDs{
				{Config: integration.Config{Name: "redis", Instances: []integration.Data{integration.Data("host: a")}}, InstanceIDs: []string{"redis:1"}},
				{Config: integration.Config{Name: "http_check", Instances: []integration.Data{integration.Data("url: b")}}, InstanceIDs: []string{"http_check:2"}},
			},
			Stats: types.CLCRunnersStats{
				"redis:1": {AverageExecutionTime: 1200, LastExecutionDate: now.UnixMilli() - 8000},
				"http_check:2": {
					AverageExecutionTime: 50, LastExecFailed: true,
					LastError: "connection refused\ntraceback...",
				},
			},
		}},
	}

	var b bytes.Buffer
	renderClusterChecks(&b, state, ClusterChecksOptions{}, now)
	out := b.String()
	assert.Regexp(t, `redis\s+redis:1\s+1.2s\s+8s ago\s+OK`, out)
	assert.Regexp(t, `http_check\s+http_check:2\s+50ms\s+-\s+ERROR`, out)
	assert.Contains(t, out, "Last error http_check:2: connection refused\n")
	assert.NotContains(t, out, "traceback")
	assert.NotContains(t, out, "host: a")
	assert.Contains(t, out, "Use --verbose")

	b.Reset()
	renderClusterChecks(&b, state, ClusterChecksOptions{CheckName: "redis"}, now)
	assert.NotContains(t, b.String(), "http_check:2")

	b.Reset()
	renderClusterChecks(&b, state, ClusterChecksOptions{Verbose: true}, now)
	out = b.String()
	assert.Contains(t, out, "host: a")
	assert.Contains(t, out, "Instance ID: redis:1 [OK]")
	assert.NotContains(t, out, "Use --verbose")
}

func TestRenderClusterChecksUnassignedCheckFilter(t *testing.T) {
	state := types.StateResponse{
		Dangling: []integration.Config{{Name: "kubernetes_state_core"}, {Name: "http_check"}},
		Nodes:    []types.StateNodeResponse{{Name: "runner-1"}},
	}

	// The unassigned section only counts the configs it shows, and is skipped when none are.
	var b bytes.Buffer
	renderClusterChecks(&b, state, ClusterChecksOptions{CheckName: "http_check"}, time.Now())
	assert.Contains(t, b.String(), "=== 1 unassigned configuration(s) ===")
	b.Reset()
	renderClusterChecks(&b, state, ClusterChecksOptions{CheckName: "redis"}, time.Now())
	assert.NotContains(t, b.String(), "unassigned")
}

func TestRenderClusterChecksInstancesAlignedWithColors(t *testing.T) {
	noColor := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = noColor }()

	state := types.StateResponse{Nodes: []types.StateNodeResponse{{
		Name: "runner-1",
		Configs: []types.ConfigWithInstanceIDs{
			{Config: integration.Config{Name: "redis", Instances: []integration.Data{integration.Data("a"), integration.Data("b")}}, InstanceIDs: []string{"redis:1", "redis:2"}},
		},
		Stats: types.CLCRunnersStats{"redis:1": {AverageExecutionTime: 1200}, "redis:2": {LastExecFailed: true}},
	}}}

	var b bytes.Buffer
	renderClusterChecks(&b, state, ClusterChecksOptions{}, time.Now())
	// Compare visible positions: strip the color escape codes.
	lines := strings.Split(regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(b.String(), ""), "\n")
	header := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "Check") && strings.Contains(l, "Avg exec") })
	require.GreaterOrEqual(t, header, 0)
	column := strings.Index(lines[header], "Avg exec")
	assert.Equal(t, column, strings.Index(lines[header+1], "1.2s"))
	assert.Equal(t, column, strings.Index(lines[header+2], "0s"))
}
