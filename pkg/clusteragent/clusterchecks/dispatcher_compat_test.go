// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

// kubeGroups declares the "kube" runner group claiming the kube-family checks.
const kubeGroups = `{"kube":["kubernetes_state_core","orchestrator"]}`

// newGroupDispatcher returns a dispatcher with the given runner groups
// declared (cluster_checks.runner_groups).
func newGroupDispatcher(t *testing.T, groups string) *dispatcher {
	t.Helper()
	configmock.New(t).SetInTest("cluster_checks.runner_groups", groups)
	return newDispatcher(taggerfxmock.SetupFakeTagger(t))
}

// registerWorker registers a worker in the given runner group ("" for a
// general worker) via the status POST path, the way real workers do.
func registerWorker(t *testing.T, d *dispatcher, name, ip string, nodeType types.NodeType, group string) {
	t.Helper()
	d.processNodeStatus(name, ip, types.NodeStatus{NodeType: nodeType, Group: group})
}

func TestParseRunnerGroups(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantChecks map[string]string
		wantGroups []string
	}{
		{"not set", "", nil, nil},
		{"invalid JSON is ignored", "not-json", nil, nil},
		{"groups", `{"kube":["kubernetes_state_core","orchestrator"],"kafka":["kafka_consumer"]}`,
			map[string]string{"kubernetes_state_core": "kube", "orchestrator": "kube", "kafka_consumer": "kafka"}, []string{"kafka", "kube"}},
		{"a check claimed twice stays with the first group in name order", `{"b":["x"],"a":["x","y"]}`,
			map[string]string{"x": "a", "y": "a"}, []string{"a", "b"}},
		{"unnamed group is ignored", `{"":["x"],"kube":["y"]}`, map[string]string{"y": "kube"}, []string{"kube"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkGroup, runnerGroups := parseRunnerGroups(tt.raw)
			assert.Equal(t, tt.wantChecks, checkGroup)
			assert.ElementsMatch(t, tt.wantGroups, slices.Collect(maps.Keys(runnerGroups)))
		})
	}
}

func TestDispatchRespectsRunnerGroups(t *testing.T) {
	tests := []struct {
		name    string
		workers map[string]string // worker name -> group
		check   string
		want    []string
	}{
		{"claimed check only goes to its group", map[string]string{"kube-1": "kube", "agent-1": ""}, "kubernetes_state_core", []string{"kube-1"}},
		{"unclaimed check only goes to general workers", map[string]string{"kube-1": "kube", "agent-1": ""}, "http_check", []string{"agent-1"}},
		{"group down: claimed check has no worker (strict)", map[string]string{"agent-1": ""}, "kubernetes_state_core", nil},
		{"worker in an unknown group runs nothing", map[string]string{"rogue": "foo"}, "http_check", nil},
		{"several group workers", map[string]string{"kube-2": "kube", "kube-1": "kube", "agent-1": ""}, "orchestrator", []string{"kube-1", "kube-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newGroupDispatcher(t, kubeGroups)
			i := 0
			for name, group := range tt.workers {
				i++
				registerWorker(t, d, name, fmt.Sprintf("10.0.0.%d", i), types.NodeTypeCLCRunner, group)
			}

			d.store.RLock()
			assert.Equal(t, tt.want, d.groupNodes(d.checkGroup[tt.check]))
			d.store.RUnlock()

			node, anyNode := d.getNodeToScheduleCheck(tt.check)
			assert.True(t, anyNode)
			if len(tt.want) == 0 {
				assert.Empty(t, node)
			} else {
				assert.Contains(t, tt.want, node)
			}
			requireNotLocked(t, d.store)
		})
	}
}

func TestAddWithNoGroupWorkerDangles(t *testing.T) {
	d := newGroupDispatcher(t, kubeGroups)
	registerWorker(t, d, "agent-1", "10.0.0.1", types.NodeTypeNodeAgent, "")

	// The kube group has no live worker: the claimed config dangles instead
	// of falling back to the general worker.
	assert.False(t, d.add(generateIntegration("kubernetes_state_core")))
	d.store.RLock()
	assert.Len(t, d.store.danglingConfigs, 1)
	d.store.RUnlock()

	// Once a kube worker registers, the dangling config is dispatched to it.
	registerWorker(t, d, "kube-1", "10.0.0.2", types.NodeTypeCLCRunner, "kube")
	dangling := d.retrieveDangling()
	require.Len(t, dangling, 1)
	assert.True(t, d.add(dangling[0]))
	d.store.RLock()
	assert.Equal(t, "kube-1", d.store.digestToNode[dangling[0].Digest()])
	d.store.RUnlock()

	requireNotLocked(t, d.store)
}

func TestUseUtilizationRebalance(t *testing.T) {
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", false)
	assert.False(t, newGroupDispatcher(t, "").useUtilizationRebalance())

	// Runner groups are declared: the busyness algorithm is not group-aware,
	// so the utilization algorithm is used regardless.
	assert.True(t, newGroupDispatcher(t, kubeGroups).useUtilizationRebalance())
}

// TestUpdateRunnersStatsFetchesWorkersWhenGroupsForceUtilization covers
// rebalance_with_utilization=false with runner groups declared: the
// utilization algorithm is forced, so worker counts must be fetched,
// otherwise every runner's utilization is 0 and nothing is ever rebalanced.
func TestUpdateRunnersStatsFetchesWorkersWhenGroupsForceUtilization(t *testing.T) {
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", false)
	for _, tt := range []struct {
		groups      string
		wantWorkers int
	}{
		{"", 0}, // busyness algorithm: worker counts aren't needed
		{kubeGroups, constants.DefaultNumWorkers},
	} {
		d := newGroupDispatcher(t, tt.groups)
		d.clcRunnersClient = &rebalanceTestClcRunnerClient{testStats: map[string]types.CLCRunnersStats{}}
		registerWorker(t, d, "runner-1", "10.0.0.1", types.NodeTypeCLCRunner, "")
		d.updateRunnersStats()

		d.store.RLock()
		assert.Equal(t, tt.wantWorkers, d.store.nodes["runner-1"].workers, "groups=%q", tt.groups)
		d.store.RUnlock()
		requireNotLocked(t, d.store)
	}
}

// rebalanceWorker is a CLC runner registered by newRebalanceDispatcher, with
// the cluster checks it currently runs (check ID -> check name).
type rebalanceWorker struct {
	name   string
	group  string
	checks map[string]string
}

// newRebalanceDispatcher returns a dispatcher ready for the utilization
// rebalance, with the given runner groups declared and each worker registered
// with its checks placed on it. Every check costs 3s per 15s interval, so a
// runner holding two is the busiest.
func newRebalanceDispatcher(t *testing.T, groups string, workers ...rebalanceWorker) *dispatcher {
	t.Helper()
	configmock.New(t).SetInTest("cluster_checks.stickiness_enabled", false)
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", true)

	d := newGroupDispatcher(t, groups)
	mockClient := &rebalanceTestClcRunnerClient{testStats: make(map[string]types.CLCRunnersStats)}
	d.clcRunnersClient = mockClient
	d.advancedDispatching.Store(true)
	d.store.active = true

	for i, w := range workers {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		registerWorker(t, d, w.name, ip, types.NodeTypeCLCRunner, w.group)

		stats := types.CLCRunnersStats{}
		d.store.Lock()
		for id, checkName := range w.checks {
			stats[id] = types.CLCRunnerStats{AverageExecutionTime: 3000, IsClusterCheck: true}
			digest := "digest-" + id
			d.store.idToDigest[checkid.ID(id)] = digest
			d.store.digestToConfig[digest] = integration.Config{Name: checkName}
			d.store.digestToNode[digest] = w.name
		}
		d.store.nodes[w.name].workers = constants.DefaultNumWorkers
		d.store.nodes[w.name].clcRunnerStats = stats
		d.store.Unlock()
		mockClient.testStats[ip] = stats
	}
	return d
}

// placement returns the current node of each digest.
func placement(d *dispatcher) map[string]string {
	d.store.RLock()
	defer d.store.RUnlock()
	return maps.Clone(d.store.digestToNode)
}

// TestRebalanceUsingUtilizationRespectsGroups verifies that the utilization
// rebalance spreads each group's checks only among that group's runners.
func TestRebalanceUsingUtilizationRespectsGroups(t *testing.T) {
	// kube checks all on kube1 and http checks all on general1 (both
	// overloaded), kube2 and general2 idle.
	d := newRebalanceDispatcher(t, `{"kube":["kube_check"]}`,
		rebalanceWorker{"kube1", "kube", map[string]string{"kube_a": "kube_check", "kube_b": "kube_check"}},
		rebalanceWorker{"kube2", "kube", nil},
		rebalanceWorker{"general1", "", map[string]string{"http_a": "http_check", "http_b": "http_check"}},
		rebalanceWorker{"general2", "", nil},
	)

	checksMoved := d.rebalanceUsingUtilization(false)
	requireNotLocked(t, d.store)

	nodes := placement(d)
	for _, digest := range []string{"digest-kube_a", "digest-kube_b"} {
		assert.Contains(t, []string{"kube1", "kube2"}, nodes[digest])
	}
	for _, digest := range []string{"digest-http_a", "digest-http_b"} {
		assert.Contains(t, []string{"general1", "general2"}, nodes[digest])
	}

	moved := map[string]bool{}
	for _, m := range checksMoved {
		moved[m.Digest] = true
	}
	// The overloaded runners were relieved: at least one move per group.
	assert.True(t, moved["digest-kube_a"] || moved["digest-kube_b"])
	assert.True(t, moved["digest-http_a"] || moved["digest-http_b"])
}
