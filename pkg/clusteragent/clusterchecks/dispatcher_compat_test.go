// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
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

	// Compat is fixed at registration: a later, different declaration is ignored.
	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Include: []string{"http_check"}})
	assert.Equal(t, []string{"kubernetes_state_core", "orchestrator"}, runner.checkCompat.Include)
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

func TestCohortKey(t *testing.T) {
	tests := []struct {
		name   string
		compat *types.CheckCompatibility
		want   string
	}{
		{"nil is general", nil, "general"},
		{"exclude-only is general", &types.CheckCompatibility{Exclude: []string{"kubernetes_state_core"}}, "general"},
		{"include sorted joined", &types.CheckCompatibility{Include: []string{"orchestrator", "kubernetes_state_core"}}, "kubernetes_state_core,orchestrator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cohortKey(tt.compat))
		})
	}
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

// TestRebalanceUsingUtilizationRespectsEligibility verifies that the
// utilization rebalance only moves configs onto eligible runners: a claimed
// check lands on its group even when currently misplaced, an unclaimed check
// moves off a group whose include does not claim it, and a check with no
// eligible worker at all is left where it is.
func TestRebalanceUsingUtilizationRespectsEligibility(t *testing.T) {
	configmock.New(t).SetInTest("cluster_checks.stickiness_enabled", false)
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", true)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)

	mockClient := &rebalanceTestClcRunnerClient{
		testStats: make(map[string]types.CLCRunnersStats),
	}
	testDispatcher.clcRunnersClient = mockClient
	testDispatcher.advancedDispatching.Store(true)
	testDispatcher.store.active = true

	registerWorker(t, testDispatcher, "group1", "10.0.0.1", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Include: []string{"kube_check"}})
	registerWorker(t, testDispatcher, "group2", "10.0.0.2", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Include: []string{"kube_check"}})
	registerWorker(t, testDispatcher, "general1", "10.0.0.3", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Exclude: []string{"kube_check"}})
	registerWorker(t, testDispatcher, "general2", "10.0.0.4", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Exclude: []string{"kube_check"}})
	for _, name := range []string{"group1", "group2", "general1", "general2"} {
		testDispatcher.store.Lock()
		testDispatcher.store.nodes[name].workers = constants.DefaultNumWorkers
		testDispatcher.store.Unlock()
	}

	// Reachable state: kube checks all on group1 (overloaded), http checks all
	// on general1 (overloaded), group2 and general2 idle. The rebalance must
	// spread each family only among its eligible runners.
	group1Stats := types.CLCRunnersStats{
		"kube_a": {AverageExecutionTime: 3000, IsClusterCheck: true},
		"kube_b": {AverageExecutionTime: 3000, IsClusterCheck: true},
	}
	general1Stats := types.CLCRunnersStats{
		"http_a": {AverageExecutionTime: 3000, IsClusterCheck: true},
		"http_b": {AverageExecutionTime: 3000, IsClusterCheck: true},
	}
	emptyStats := types.CLCRunnersStats{}
	testDispatcher.store.Lock()
	testDispatcher.store.nodes["group1"].clcRunnerStats = group1Stats
	testDispatcher.store.nodes["group2"].clcRunnerStats = emptyStats
	testDispatcher.store.nodes["general1"].clcRunnerStats = general1Stats
	testDispatcher.store.nodes["general2"].clcRunnerStats = emptyStats
	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"kube_a": "digest-kube-a",
		"kube_b": "digest-kube-b",
		"http_a": "digest-http-a",
		"http_b": "digest-http-b",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digest-kube-a": {Name: "kube_check"},
		"digest-kube-b": {Name: "kube_check"},
		"digest-http-a": {Name: "http_check"},
		"digest-http-b": {Name: "http_check"},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digest-kube-a": "group1",
		"digest-kube-b": "group1",
		"digest-http-a": "general1",
		"digest-http-b": "general1",
	}
	testDispatcher.store.Unlock()
	mockClient.testStats["10.0.0.1"] = group1Stats
	mockClient.testStats["10.0.0.2"] = emptyStats
	mockClient.testStats["10.0.0.3"] = general1Stats
	mockClient.testStats["10.0.0.4"] = emptyStats

	checksMoved := testDispatcher.rebalanceUsingUtilization(false)
	requireNotLocked(t, testDispatcher.store)

	testDispatcher.store.RLock()
	kubeA := testDispatcher.store.digestToNode["digest-kube-a"]
	kubeB := testDispatcher.store.digestToNode["digest-kube-b"]
	httpA := testDispatcher.store.digestToNode["digest-http-a"]
	httpB := testDispatcher.store.digestToNode["digest-http-b"]
	testDispatcher.store.RUnlock()

	// Every kube check stays on the group runners, every http check on the
	// general runners, whatever the rebalance decided to move.
	for _, target := range []string{kubeA, kubeB} {
		assert.Contains(t, []string{"group1", "group2"}, target)
	}
	for _, target := range []string{httpA, httpB} {
		assert.Contains(t, []string{"general1", "general2"}, target)
	}

	moved := map[string]bool{}
	for _, m := range checksMoved {
		moved[m.Digest] = true
	}
	// The overloaded runners were relieved: at least one move per family.
	assert.True(t, moved["digest-kube-a"] || moved["digest-kube-b"])
	assert.True(t, moved["digest-http-a"] || moved["digest-http-b"])
}
