// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Benchmark scenario: 300 CLC runners, 5000 cluster check configs, 10
// isolation groups (9 dedicated groups + the default fleet), 30 runners each.
// 100 distinct check names: each dedicated group claims 5 of them, the default
// fleet excludes the 45 claimed names and runs the other 55.
const (
	benchRunners    = 300
	benchConfigs    = 5000
	benchGroups     = 10
	benchCheckNames = 100
	benchClaimed    = 5 // check names claimed per dedicated group
)

type benchLayout struct {
	compat bool // declare isolation groups; false = legacy, no compat at all
	skewed bool // pile each group's configs on 2 runners so the rebalance moves many
}

// newBenchDispatcher builds a dispatcher with the benchmark scenario loaded.
func newBenchDispatcher(b *testing.B, layout benchLayout) *dispatcher {
	b.Helper()
	rng := rand.New(rand.NewSource(42))

	d := newDispatcher(taggerfxmock.SetupFakeTagger(b))
	log.SetupLogger(log.Default(), "off")
	mockClient := &rebalanceTestClcRunnerClient{testStats: make(map[string]types.CLCRunnersStats)}
	d.clcRunnersClient = mockClient
	d.advancedDispatching.Store(true)
	d.store.active = true

	// Group g claims check names [g*5, g*5+5) for g < 9; the default fleet
	// (group 9) excludes all 45 claimed names.
	checkName := func(i int) string { return fmt.Sprintf("check_%03d", i) }
	var claimedAll []string
	groupCompat := make([]*types.CheckCompatibility, benchGroups)
	for g := 0; g < benchGroups-1; g++ {
		var include []string
		for i := g * benchClaimed; i < (g+1)*benchClaimed; i++ {
			include = append(include, checkName(i))
		}
		claimedAll = append(claimedAll, include...)
		groupCompat[g] = &types.CheckCompatibility{Include: include}
	}
	groupCompat[benchGroups-1] = &types.CheckCompatibility{Exclude: claimedAll}

	runnersByGroup := make([][]string, benchGroups)
	for r := 0; r < benchRunners; r++ {
		g := r % benchGroups
		name := fmt.Sprintf("runner-%03d", r)
		runnersByGroup[g] = append(runnersByGroup[g], name)
		var compat *types.CheckCompatibility
		if layout.compat {
			compat = groupCompat[g]
		}
		d.processNodeStatus(name, fmt.Sprintf("10.0.%d.%d", r/250, r%250), types.NodeStatus{NodeType: types.NodeTypeCLCRunner, CheckCompatibility: compat})
	}

	stats := make(map[string]types.CLCRunnersStats, benchRunners)
	d.store.Lock()
	for name := range d.store.nodes {
		stats[name] = types.CLCRunnersStats{}
	}
	for c := 0; c < benchConfigs; c++ {
		n := rng.Intn(benchCheckNames)
		g := benchGroups - 1
		if n < len(claimedAll) {
			g = n / benchClaimed
		}
		if !layout.compat {
			g = rng.Intn(benchGroups) // legacy: groups don't exist, spread anywhere
		}
		candidates := runnersByGroup[g]
		if layout.skewed {
			candidates = candidates[:2]
		}
		runner := candidates[rng.Intn(len(candidates))]

		id := fmt.Sprintf("check-%05d", c)
		digest := fmt.Sprintf("digest-%05d", c)
		config := integration.Config{
			Name:      checkName(n),
			Instances: []integration.Data{integration.Data(fmt.Sprintf("min_collection_interval: 15\nid: %d", c))},
		}
		d.store.idToDigest[checkid.ID(id)] = digest
		d.store.digestToConfig[digest] = config
		d.store.digestToNode[digest] = runner
		d.store.nodes[runner].digestToConfig[digest] = config
		// 50ms-2s per 15s run: ~0.2 of a worker on average.
		stats[runner][id] = types.CLCRunnerStats{AverageExecutionTime: 50 + rng.Intn(1950), IsClusterCheck: true}
	}
	for name, node := range d.store.nodes {
		node.workers = constants.DefaultNumWorkers
		node.clcRunnerStats = stats[name]
		mockClient.testStats[node.clientIP] = stats[name]
	}
	d.store.Unlock()
	return d
}

func benchSetup(b *testing.B) {
	configmock.New(b).SetInTest("cluster_checks.stickiness_enabled", true)
	configmock.New(b).SetInTest("cluster_checks.rebalance_with_utilization", true)
	log.SetupLogger(log.Default(), "off")
}

// BenchmarkRebalanceUsingUtilization measures one full utilization rebalance
// (stats refresh, current distribution, cohort partitioning, greedy placement
// and applying the moves).
func BenchmarkRebalanceUsingUtilization(b *testing.B) {
	for _, bc := range []struct {
		name   string
		layout benchLayout
	}{
		{"legacy-no-compat/balanced", benchLayout{compat: false}},
		{"10-groups/balanced", benchLayout{compat: true}},
		{"legacy-no-compat/skewed", benchLayout{compat: false, skewed: true}},
		{"10-groups/skewed", benchLayout{compat: true, skewed: true}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			benchSetup(b)
			b.ReportAllocs()
			moves := 0
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				d := newBenchDispatcher(b, bc.layout)
				b.StartTimer()
				moves += len(d.rebalanceUsingUtilization(false))
			}
			b.ReportMetric(float64(moves)/float64(b.N), "moves/op")
		})
	}
}

// BenchmarkCohortPartitioning isolates the cost this PR adds on top of the
// legacy rebalance: computing each config's eligible runners and cohort key.
func BenchmarkCohortPartitioning(b *testing.B) {
	benchSetup(b)
	d := newBenchDispatcher(b, benchLayout{compat: true})
	current := d.currentDistribution()

	b.Run("per-config", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			d.store.RLock()
			for _, config := range current.Configs {
				_ = d.cohortKey(d.eligibleNodes(config.CheckName))
			}
			d.store.RUnlock()
		}
	})

	// Eligibility only depends on the check name: computing it once per
	// distinct name (100) instead of once per config (5000).
	b.Run("cached-per-check-name", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			d.store.RLock()
			keys := make(map[string]string)
			for _, config := range current.Configs {
				if _, ok := keys[config.CheckName]; !ok {
					keys[config.CheckName] = d.cohortKey(d.eligibleNodes(config.CheckName))
				}
			}
			d.store.RUnlock()
		}
	})
}

// BenchmarkGetNodeToScheduleCheck measures dispatching one new config.
func BenchmarkGetNodeToScheduleCheck(b *testing.B) {
	for _, compat := range []bool{false, true} {
		b.Run(fmt.Sprintf("compat=%v", compat), func(b *testing.B) {
			benchSetup(b)
			d := newBenchDispatcher(b, benchLayout{compat: compat})
			b.ReportAllocs()
			for b.Loop() {
				d.getNodeToScheduleCheck("check_001")
			}
		})
	}
}
