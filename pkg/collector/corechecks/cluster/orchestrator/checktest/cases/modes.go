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
)

func init() { suite.Register("Modes", modes) }

// modes runs the tests of the deployment modes: leader election and the cluster check runner.
func modes(t *testing.T, s *suite.Suite) {
	s.RunTests(t,
		// A follower sends nothing and queues no deletion. When it leads again, it sends the deletions that follow.
		suite.Test{
			Name: "LeadershipLostAndRegained",
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.Collected),
				},
				{
					Name:   "copy every object",
					Change: suite.CopyAll(suite.Collected),
					Expect: suite.Live(suite.Copies),
				},
				{
					Name:   "lose the leadership",
					Change: suite.SetLeader(false),
					Expect: suite.NoObjects,
				},
				{
					Name:   "delete the originals while following",
					Change: suite.DeleteAll(suite.Originals),
					Expect: suite.NoObjects,
				},
				{
					Name:   "regain the leadership",
					Change: suite.SetLeader(true),
					Expect: suite.NoObjects,
				},
				{
					Name:   "delete the copies while leading",
					Change: suite.DeleteAll(suite.Copies),
					Expect: suite.Deleted(suite.Copies),
				},
			},
		},
		// The Cluster Agent check sends nothing without leader election.
		suite.Test{
			Name: "LeaderElectionDisabled",
			Steps: []suite.Step{
				{
					Name:   "run without leader election",
					Change: func(_ *testing.T, env *suite.Env) { env.Config.SetInTest("leader_election", false) },
					Run:    suite.RunFails,
					Expect: suite.NoObjects,
				},
			},
		},
		// A cluster check runner adds the infrastructure mode tags to the payloads that follow their arrival, as the
		// tagger stream of the Cluster Agent can connect after the check is configured.
		suite.Test{
			Name:  "ClusterCheckRunnerLateInfraTags",
			Setup: suite.Setup{Mode: suite.ClusterCheckRunner},
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.Collected),
				},
				{
					Name: "the infrastructure mode tags arrive, then every object changes",
					Change: func(t *testing.T, env *suite.Env) {
						env.Tagger.SetTags(taggertypes.GetInfraTagsEntityID(), "remote", []string{"infra_mode:cloud_cost_only"}, nil, nil, nil)
						suite.UpdateAll(suite.Collected)(t, env)
					},
					Expect: func(t *testing.T, env *suite.Env) suite.Payloads {
						return suite.Live(suite.Collected)(t, env).WithMessageTags("infra_mode:cloud_cost_only")
					},
				},
			},
		},
	)
}
