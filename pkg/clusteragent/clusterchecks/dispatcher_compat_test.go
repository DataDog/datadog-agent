// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"maps"
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

func kubeCompat() *types.CheckCompatibility {
	return &types.CheckCompatibility{Include: []string{"kubernetes_state_core", "orchestrator"}}
}

// registerWorker registers a worker with the given name, IP and advertised
// compatibility via the status POST path, the way real workers do.
func registerWorker(t *testing.T, d *dispatcher, name, ip string, nodeType types.NodeType, compat *types.CheckCompatibility) {
	t.Helper()
	d.processNodeStatus(name, ip, types.NodeStatus{NodeType: nodeType, CheckCompatibility: compat})
}

func TestProcessNodeStatusStoresCheckCompatibility(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner, kubeCompat())
	registerWorker(t, dispatcher, "agent1", "10.0.0.2", types.NodeTypeNodeAgent, nil)

	requireNotLocked(t, dispatcher.store)

	runner, found := dispatcher.store.getNodeStore("runner1")
	require.True(t, found)
	require.NotNil(t, runner.checkCompat)
	assert.Equal(t, []string{"kubernetes_state_core", "orchestrator"}, runner.checkCompat.Include)

	agent, found := dispatcher.store.getNodeStore("agent1")
	require.True(t, found)
	assert.Nil(t, agent.checkCompat)

	// A later, different declaration from the same worker name replaces it.
	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Include: []string{"http_check"}})
	assert.Equal(t, []string{"http_check"}, runner.checkCompat.Include)
	assert.Equal(t, "include=http_check", runner.signature)
}

// TestDeclarationChangeRedispatchesRefusedConfigs covers a node agent
// restarting under the same name (DaemonSet rollout) with a new exclude list:
// configs it now refuses are unassigned and re-dispatched elsewhere, the
// others stay put.
func TestDeclarationChangeRedispatchesRefusedConfigs(t *testing.T) {
	dispatcher := newDispatcher(taggerfxmock.SetupFakeTagger(t))
	registerWorker(t, dispatcher, "agent1", "10.0.0.1", types.NodeTypeNodeAgent, &types.CheckCompatibility{Exclude: []string{"orchestrator"}})
	registerWorker(t, dispatcher, "kube-runner", "10.0.0.2", types.NodeTypeCLCRunner, kubeCompat())

	ksm := generateIntegration("kubernetes_state_core")
	http := generateIntegration("http_check")
	dispatcher.addConfig(ksm, "agent1")
	dispatcher.addConfig(http, "agent1")

	// The restarted node agent now also excludes kubernetes_state_core.
	agent, _ := dispatcher.store.getNodeStore("agent1")
	lastChange := agent.lastConfigChange
	registerWorker(t, dispatcher, "agent1", "10.0.0.1", types.NodeTypeNodeAgent, &types.CheckCompatibility{Exclude: []string{"kubernetes_state_core", "orchestrator"}})
	requireNotLocked(t, dispatcher.store)

	dispatcher.store.RLock()
	assert.NotContains(t, dispatcher.store.digestToNode, ksm.Digest())
	assert.Contains(t, dispatcher.store.danglingConfigs, ksm.Digest())
	assert.Equal(t, "agent1", dispatcher.store.digestToNode[http.Digest()])
	dispatcher.store.RUnlock()
	agent.RLock()
	assert.NotContains(t, agent.digestToConfig, ksm.Digest())
	assert.Contains(t, agent.digestToConfig, http.Digest())
	// The node agent is told to re-poll, so it stops running the refused check.
	assert.Greater(t, agent.lastConfigChange, lastChange)
	agent.RUnlock()

	// The dangling config lands on the only worker that accepts it.
	danglingConfigs := dispatcher.retrieveDangling()
	require.Len(t, danglingConfigs, 1)
	assert.True(t, dispatcher.add(danglingConfigs[0]))
	dispatcher.store.RLock()
	assert.Equal(t, "kube-runner", dispatcher.store.digestToNode[ksm.Digest()])
	dispatcher.store.RUnlock()

	// Re-sending the same declaration is a no-op.
	lastChange = agent.lastConfigChange
	registerWorker(t, dispatcher, "agent1", "10.0.0.1", types.NodeTypeNodeAgent, &types.CheckCompatibility{Exclude: []string{"orchestrator", "kubernetes_state_core"}})
	assert.Equal(t, lastChange, agent.lastConfigChange)
	requireNotLocked(t, dispatcher.store)
}

func TestEligibleNodes(t *testing.T) {
	kubeExclude := &types.CheckCompatibility{Exclude: []string{"kubernetes_state_core", "orchestrator"}}
	type worker struct {
		name   string
		compat *types.CheckCompatibility
	}
	tests := []struct {
		name     string
		workers  []worker
		check    string
		expected []string
	}{
		{"claimed check goes to its group only", []worker{{"runner1", kubeCompat()}, {"agent1", kubeExclude}}, "kubernetes_state_core", []string{"runner1"}},
		{"unclaimed check skips the group", []worker{{"runner1", kubeCompat()}, {"agent1", kubeExclude}}, "http_check", []string{"agent1"}},
		{"group down: claimed check dangles", []worker{{"agent1", kubeExclude}}, "kubernetes_state_core", nil},
		{"legacy worker is unrestricted", []worker{{"agent1", kubeExclude}, {"legacy", nil}}, "kubernetes_state_core", []string{"legacy"}},
		{"results are sorted", []worker{{"b", nil}, {"a", nil}}, "http_check", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := newDispatcher(taggerfxmock.SetupFakeTagger(t))
			for i, w := range tt.workers {
				registerWorker(t, dispatcher, w.name, fmt.Sprintf("10.0.0.%d", i+1), types.NodeTypeCLCRunner, w.compat)
			}
			// Load on the first worker must not attract an ineligible check.
			dispatcher.addConfig(generateIntegration("other"), tt.workers[0].name)

			dispatcher.store.RLock()
			assert.Equal(t, tt.expected, dispatcher.eligibleNodes(tt.check))
			dispatcher.store.RUnlock()

			node, anyNode := dispatcher.getNodeToScheduleCheck(tt.check)
			assert.True(t, anyNode)
			if len(tt.expected) == 0 {
				assert.Empty(t, node)
			} else {
				assert.Contains(t, tt.expected, node)
			}
			requireNotLocked(t, dispatcher.store)
		})
	}
}

func TestAddWithNoEligibleWorkerDangles(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	registerWorker(t, dispatcher, "default-runner", "10.0.0.1", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Exclude: []string{"kubernetes_state_core"}})

	// The only live worker refuses the check: the config must dangle rather
	// than being dispatched to an ineligible worker.
	assert.False(t, dispatcher.add(generateIntegration("kubernetes_state_core")))
	dispatcher.store.RLock()
	dangling := len(dispatcher.store.danglingConfigs)
	dispatcher.store.RUnlock()
	assert.Equal(t, 1, dangling)

	// Once an eligible worker appears, the dangling config can be re-dispatched.
	registerWorker(t, dispatcher, "kube-runner", "10.0.0.2", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Include: []string{"kubernetes_state_core"}})
	danglingConfigs := dispatcher.retrieveDangling()
	require.Len(t, danglingConfigs, 1)
	assert.True(t, dispatcher.add(danglingConfigs[0]))

	dispatcher.store.RLock()
	target := dispatcher.store.digestToNode[danglingConfigs[0].Digest()]
	dispatcher.store.RUnlock()
	assert.Equal(t, "kube-runner", target)

	requireNotLocked(t, dispatcher.store)
}

func TestDeclarationSignature(t *testing.T) {
	tests := []struct {
		name   string
		compat *types.CheckCompatibility
		want   string
	}{
		{"nil is unrestricted", nil, "unrestricted"},
		{"empty lists are unrestricted", &types.CheckCompatibility{Include: []string{}, Exclude: []string{}}, "unrestricted"},
		{"include only", &types.CheckCompatibility{Include: []string{"orchestrator", "kubernetes_state_core"}}, "include=kubernetes_state_core,orchestrator"},
		{"exclude only", &types.CheckCompatibility{Exclude: []string{"foo"}}, "exclude=foo"},
		{"include wins over exclude", &types.CheckCompatibility{Include: []string{"a"}, Exclude: []string{"c"}}, "include=a"},
		{"sorted and deduplicated", &types.CheckCompatibility{Exclude: []string{"d", "c", "c"}}, "exclude=c,d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, declarationSignature(tt.compat))
		})
	}
}

func TestCohortKey(t *testing.T) {
	dispatcher := newDispatcher(taggerfxmock.SetupFakeTagger(t))
	kube := &types.CheckCompatibility{Include: []string{"kube_check"}}
	general := &types.CheckCompatibility{Exclude: []string{"kube_check"}}
	registerWorker(t, dispatcher, "kube-1", "10.0.0.1", types.NodeTypeCLCRunner, kube)
	registerWorker(t, dispatcher, "kube-2", "10.0.0.2", types.NodeTypeCLCRunner, kube)
	registerWorker(t, dispatcher, "general-1", "10.0.0.3", types.NodeTypeCLCRunner, general)
	registerWorker(t, dispatcher, "legacy", "10.0.0.4", types.NodeTypeNodeAgent, nil)

	dispatcher.store.RLock()
	defer dispatcher.store.RUnlock()
	// Same-declaration workers collapse to one signature, whatever the pod names.
	assert.Equal(t, "include=kube_check", dispatcher.cohortKey([]string{"kube-1", "kube-2"}))
	assert.Equal(t, dispatcher.cohortKey([]string{"kube-1"}), dispatcher.cohortKey([]string{"kube-2"}))
	// Distinct declarations are all kept, sorted.
	assert.Equal(t, "exclude=kube_check | unrestricted", dispatcher.cohortKey([]string{"legacy", "general-1"}))
}

func TestUseUtilizationRebalance(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	// No compat declared anywhere: the configured algorithm applies (pure
	// additive behavior preserved).
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", false)
	assert.False(t, dispatcher.useUtilizationRebalance())
	registerWorker(t, dispatcher, "agent1", "10.0.0.2", types.NodeTypeNodeAgent, nil)
	assert.False(t, dispatcher.useUtilizationRebalance())

	// A runner group declares compatibility: the busyness algorithm is not
	// compatibility-aware, so the utilization algorithm is used regardless.
	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner, kubeCompat())
	assert.True(t, dispatcher.useUtilizationRebalance())

	requireNotLocked(t, dispatcher.store)
}

// rebalanceWorker is a CLC runner registered by newRebalanceDispatcher, with
// the cluster checks it currently runs (check ID -> check name).
type rebalanceWorker struct {
	name   string
	compat *types.CheckCompatibility
	checks map[string]string
}

// newRebalanceDispatcher returns a dispatcher ready for the utilization
// rebalance, with each worker registered and its checks placed on it. Every
// check costs 3s per 15s interval, so a runner holding two is the busiest.
func newRebalanceDispatcher(t *testing.T, workers ...rebalanceWorker) *dispatcher {
	t.Helper()
	configmock.New(t).SetInTest("cluster_checks.stickiness_enabled", false)
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", true)

	d := newDispatcher(taggerfxmock.SetupFakeTagger(t))
	mockClient := &rebalanceTestClcRunnerClient{testStats: make(map[string]types.CLCRunnersStats)}
	d.clcRunnersClient = mockClient
	d.advancedDispatching.Store(true)
	d.store.active = true

	for i, w := range workers {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		registerWorker(t, d, w.name, ip, types.NodeTypeCLCRunner, w.compat)

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

// TestRebalanceUsingUtilizationRespectsEligibility verifies that the
// utilization rebalance spreads each check family only among its eligible
// runners.
func TestRebalanceUsingUtilizationRespectsEligibility(t *testing.T) {
	// kube checks all on group1 and http checks all on general1 (both
	// overloaded), group2 and general2 idle.
	kubeGroup := &types.CheckCompatibility{Include: []string{"kube_check"}}
	general := &types.CheckCompatibility{Exclude: []string{"kube_check"}}
	d := newRebalanceDispatcher(t,
		rebalanceWorker{"group1", kubeGroup, map[string]string{"kube_a": "kube_check", "kube_b": "kube_check"}},
		rebalanceWorker{"group2", kubeGroup, nil},
		rebalanceWorker{"general1", general, map[string]string{"http_a": "http_check", "http_b": "http_check"}},
		rebalanceWorker{"general2", general, nil},
	)

	checksMoved := d.rebalanceUsingUtilization(false)
	requireNotLocked(t, d.store)

	nodes := placement(d)
	for _, digest := range []string{"digest-kube_a", "digest-kube_b"} {
		assert.Contains(t, []string{"group1", "group2"}, nodes[digest])
	}
	for _, digest := range []string{"digest-http_a", "digest-http_b"} {
		assert.Contains(t, []string{"general1", "general2"}, nodes[digest])
	}

	moved := map[string]bool{}
	for _, m := range checksMoved {
		moved[m.Digest] = true
	}
	// The overloaded runners were relieved: at least one move per family.
	assert.True(t, moved["digest-kube_a"] || moved["digest-kube_b"])
	assert.True(t, moved["digest-http_a"] || moved["digest-http_b"])
}

// TestRebalanceCohortsKeyedByEligibleSet is the regression test for workers
// whose declarations look alike (both exclude-only, so both labeled
// "general") but accept different checks: each check must only ever be
// placed on a runner that accepts it.
func TestRebalanceCohortsKeyedByEligibleSet(t *testing.T) {
	// a refuses foo, b refuses bar. foo can only run on b, bar only on a,
	// http on both. b is overloaded, a is idle.
	d := newRebalanceDispatcher(t,
		rebalanceWorker{"a", &types.CheckCompatibility{Exclude: []string{"foo"}}, map[string]string{"bar_a": "bar"}},
		rebalanceWorker{"b", &types.CheckCompatibility{Exclude: []string{"bar"}}, map[string]string{
			"foo_a": "foo", "foo_b": "foo", "http_a": "http_check", "http_b": "http_check",
		}},
	)

	// Map iteration picks the cohort's first config at random: repeat so a
	// regression would fail reliably rather than intermittently.
	for range 20 {
		d.rebalanceUsingUtilization(true)
		requireNotLocked(t, d.store)

		nodes := placement(d)
		assert.Equal(t, "b", nodes["digest-foo_a"])
		assert.Equal(t, "b", nodes["digest-foo_b"])
		assert.Equal(t, "a", nodes["digest-bar_a"])
	}
}
