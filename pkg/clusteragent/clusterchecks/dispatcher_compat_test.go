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
	status := types.NodeStatus{
		LastChange:         0,
		NodeType:           nodeType,
		CheckCompatibility: compat,
	}
	d.processNodeStatus(name, ip, status)
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

func TestPlacementCandidates(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner, kubeCompat())
	registerWorker(t, dispatcher, "agent1", "10.0.0.2", types.NodeTypeNodeAgent,
		&types.CheckCompatibility{Exclude: []string{"kubernetes_state_core", "orchestrator"}})

	// Claimed check: only the group runner admits it. The node agent declared
	// the group exclude union (the operator's strict-isolation setup).
	assert.Equal(t, []string{"runner1"}, dispatcher.placementCandidates("kubernetes_state_core"))

	// Unclaimed check: the node agent is eligible (only group checks excluded).
	assert.Equal(t, []string{"agent1"}, dispatcher.placementCandidates("http_check"))

	// Group down: no eligible worker remains, the config dangles (strict).
	dispatcher.store.Lock()
	delete(dispatcher.store.nodes, "runner1")
	dispatcher.store.Unlock()
	assert.Empty(t, dispatcher.placementCandidates("kubernetes_state_core"))
	// ...but unclaimed checks still have the node agent.
	assert.Equal(t, []string{"agent1"}, dispatcher.placementCandidates("http_check"))

	// A legacy node agent (no compat declared) is eligible for everything:
	// eligibility is the only rule, strict isolation comes from the
	// operator-propagated declarations, not from the dispatcher.
	registerWorker(t, dispatcher, "legacy-agent", "10.0.0.3", types.NodeTypeNodeAgent, nil)
	assert.Equal(t, []string{"legacy-agent"}, dispatcher.placementCandidates("kubernetes_state_core"))

	// Runner-only pool where the sole runner refuses the claimed check
	// (exclude union, the operator's default CCR setup): no eligible worker at all.
	dispatcher.store.Lock()
	delete(dispatcher.store.nodes, "agent1")
	delete(dispatcher.store.nodes, "legacy-agent")
	dispatcher.store.nodes["default-runner"] = newNodeStore("default-runner", "10.0.0.4")
	dispatcher.store.Unlock()
	registerWorker(t, dispatcher, "default-runner", "10.0.0.4", types.NodeTypeCLCRunner,
		&types.CheckCompatibility{Exclude: []string{"kubernetes_state_core", "orchestrator"}})
	assert.Empty(t, dispatcher.placementCandidates("kubernetes_state_core"))
	// ...but the default runner is eligible for unclaimed checks.
	assert.Equal(t, []string{"default-runner"}, dispatcher.placementCandidates("http_check"))

	requireNotLocked(t, dispatcher.store)
}

func TestGetNodeWithLessChecksRespectsEligibility(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner, kubeCompat())
	registerWorker(t, dispatcher, "agent1", "10.0.0.2", types.NodeTypeNodeAgent,
		&types.CheckCompatibility{Exclude: []string{"kubernetes_state_core", "orchestrator"}})

	// Claimed check: only the group runner is eligible, the node agent's
	// check count is irrelevant.
	dispatcher.addConfig(generateIntegration("http_check"), "agent1")
	assert.Equal(t, "runner1", dispatcher.getNodeWithLessChecks("kubernetes_state_core"))

	// Unclaimed check: the node agent is the only eligible worker.
	assert.Equal(t, "agent1", dispatcher.getNodeWithLessChecks("http_check"))

	// Group down: no eligible worker -> empty string, the config dangles.
	dispatcher.store.Lock()
	delete(dispatcher.store.nodes, "runner1")
	dispatcher.store.Unlock()
	assert.Equal(t, "", dispatcher.getNodeWithLessChecks("kubernetes_state_core"))

	requireNotLocked(t, dispatcher.store)
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

func TestAnyCompatDeclared(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	assert.False(t, dispatcher.anyCompatDeclared())

	registerWorker(t, dispatcher, "agent1", "10.0.0.2", types.NodeTypeNodeAgent, nil)
	assert.False(t, dispatcher.anyCompatDeclared())

	registerWorker(t, dispatcher, "runner1", "10.0.0.1", types.NodeTypeCLCRunner, kubeCompat())
	assert.True(t, dispatcher.anyCompatDeclared())

	requireNotLocked(t, dispatcher.store)
}

func TestUseUtilizationRebalance(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	// No compat declared anywhere: the configured algorithm applies (pure
	// additive behavior preserved).
	configmock.New(t).SetInTest("cluster_checks.rebalance_with_utilization", false)
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

func TestUtilizationStdDevWeightedDegeneratesToGlobal(t *testing.T) {
	dist := newConfigsDistribution(map[string]int{"a": 4, "b": 4, "c": 4, "d": 4}, false, 4, 1, 0.05)
	// Legacy placement: no eligibility info anywhere.
	dist.addConfig("d1", "check1", 2, "a", false)
	dist.addConfig("d2", "check2", 1, "a", false)
	dist.addConfig("d3", "check3", 4, "c", false)

	assert.InDelta(t, dist.utilizationStdDev(), dist.utilizationStdDevWeighted(), 1e-9)
}

// TestUtilizationStdDevWeightedIgnoresIsolationSkew verifies the RFC 3a
// property that a deliberately skewed runner group does not read as a global
// imbalance: with two cohorts each perfectly balanced within itself but with
// different utilization levels (group heavy, general light), the weighted
// stddev is low while the global stddev is high.
func TestUtilizationStdDevWeightedIgnoresIsolationSkew(t *testing.T) {
	// group cohort: runners a, b (heavy: 3 workers used each). general cohort:
	// runners c, d (light: 1 worker used each). Every cohort is perfectly
	// balanced within itself; the global distribution is skewed.
	dist := newConfigsDistribution(map[string]int{"a": 4, "b": 4, "c": 4, "d": 4}, false, 4, 1, 0.05)
	groupCohort := []string{"a", "b"}
	generalCohort := []string{"c", "d"}

	for i := 0; i < 3; i++ {
		for _, runner := range []string{"a", "b"} {
			dist.addConfigWithEligibility(groupCohort, fmt.Sprintf("group-%d-%s", i, runner), "kube_check", 1, runner, false)
		}
	}
	for _, runner := range []string{"c", "d"} {
		dist.addConfigWithEligibility(generalCohort, "general-"+runner, "http_check", 1, runner, false)
	}

	// Each cohort is balanced: weighted stddev is zero.
	assert.InDelta(t, 0.0, dist.utilizationStdDevWeighted(), 1e-9)
	// The global distribution is skewed: plain stddev is high.
	assert.Greater(t, dist.utilizationStdDev(), 0.2)
}
