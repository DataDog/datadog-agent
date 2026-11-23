// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"testing"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func init() { suite.Register("Modes", modes) }

// modes runs the tests of the deployment modes: leader election and the cluster check runner.
func modes(t *testing.T, cs *suite.Suite) {
	// The leadership case deletes each object once: the fixtures while following, and a copy of each while leading.
	var originals, copies []suite.ScenarioObject
	cs.RunCases(t,
		// A follower sends nothing and queues no deletion, and the check sends the deletions that follow when it leads
		// again.
		suite.Case{Name: "LeadershipLostAndRegained", Steps: []suite.Step{
			suite.FirstRun,
			{
				Name: "add a copy of every object",
				Act: func(t *testing.T, sc *suite.Scenario) {
					originals = sc.Collected()
					copies = sc.AddCopies(t, originals)
				},
				// The copies of the nodes change the cluster.
				Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
					return sc.LiveOf(t, copies).With(sc.ClusterPayloads())
				},
			},
			{Name: "lose the leadership", Act: func(_ *testing.T, sc *suite.Scenario) { sc.Mocks.Leader = false }, Want: suite.Nothing},
			{
				Name: "delete every object while following",
				Act:  func(t *testing.T, sc *suite.Scenario) { sc.DeleteAll(t, originals) },
				Want: suite.Nothing,
			},
			{
				Name: "regain the leadership",
				Act:  func(_ *testing.T, sc *suite.Scenario) { sc.Mocks.Leader = true },
				// The nodes deleted while following changed the cluster.
				Want: func(_ *testing.T, sc *suite.Scenario) suite.ExpectedPayloads { return sc.ClusterPayloads() },
			},
			{
				Name: "delete every copy while leading",
				Act:  func(t *testing.T, sc *suite.Scenario) { sc.DeleteAll(t, copies) },
				Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
					return sc.TerminatedOf(t, copies).With(sc.ClusterPayloads())
				},
			},
		}},
		// The Cluster Agent check sends nothing without leader election.
		suite.Case{Name: "LeaderElectionDisabled", Steps: []suite.Step{{
			Name: "run without leader election",
			Act:  func(*testing.T, *suite.Scenario) { pkgconfigsetup.Datadog().SetInTest("leader_election", false) },
			Run:  suite.RunFails,
			Want: suite.Nothing,
		}}},
		// A cluster check runner adds the infrastructure mode tags to the payloads that follow their arrival, as the
		// tagger stream of the Cluster Agent can connect after the check is configured.
		suite.Case{Name: "ClusterCheckRunnerLateInfraTags", Variants: suite.PipelineVariants(suite.ClusterCheckRunner), Steps: []suite.Step{
			suite.FirstRun,
			{
				Name: "the infrastructure mode tags arrive, then every object changes",
				Act: func(t *testing.T, sc *suite.Scenario) {
					sc.Mocks.Tagger.SetTags(taggertypes.GetInfraTagsEntityID(), "remote", []string{"infra_mode:cloud_cost_only"}, nil, nil, nil)
					for _, o := range sc.Collected() {
						sc.Update(t, suite.UpdatedCopy(o.Object, "2"))
					}
				},
				Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
					return sc.LiveOf(t, sc.Collected()).WithMessageTags("infra_mode:cloud_cost_only")
				},
			},
		}},
	)
}
