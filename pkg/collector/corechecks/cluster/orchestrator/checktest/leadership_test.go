// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func TestLeadership(t *testing.T) {
	// A follower returns before it starts its informers, so changes to the cluster cannot make it send anything.
	t.Run("follower_sends_nothing", func(t *testing.T) {
		env := suite.Setup(t, suite.WithMode(suite.ClusterAgentFollower))

		env.AssertPop(t)
		assert.Positive(t, env.LeaderElections())
	})

	// A follower queues no deletion, and learns that it lost the leadership when it runs. When it leads again, it sends
	// the deletions that follow.
	t.Run("deletions_while_following_are_dropped", func(t *testing.T) {
		env := suite.Setup(t)
		originals := env.UIDs()

		env.AssertPop(t, firstRun(originals)...)
		copies := env.AddCopies(t)
		env.AssertPop(t, firstRun(copies)...)
		env.SetLeader(false)
		env.AssertPop(t)
		env.DeleteUIDs(t, originals...)
		env.WaitForDeletions(t, deletion(originals)...)
		env.AssertPop(t)
		// The nodes changed while following, so the check sends the cluster again.
		env.SetLeader(true)
		env.AssertPop(t, "live cluster")
		env.DeleteUIDs(t, copies...)
		expected := append(deletion(copies), "live cluster")
		env.AssertPop(t, expected...)
	})

	// The check sends the deletions that it queued while leading at its first run as a follower.
	t.Run("deletions_queued_while_leading_are_sent_when_the_leadership_is_lost", func(t *testing.T) {
		env := suite.Setup(t)
		uids := env.UIDs()

		env.AssertPop(t, firstRun(uids)...)
		env.DeleteAll(t)
		env.WaitForDeletions(t, deletion(uids)...)
		env.SetLeader(false)
		env.AssertPop(t, deletion(uids)...)
	})

	// A cluster check dispatched without skip_leader_election runs the election, and only sends when it leads.
	t.Run("cluster_check_runner_without_skip_runs_the_election", func(t *testing.T) {
		env := suite.Setup(t, suite.WithMode(suite.ClusterCheckRunner), suite.WithoutSkipLeaderElection())
		env.SetLeader(false)

		env.AssertPop(t)
		assert.Positive(t, env.LeaderElections())
		env.SetLeader(true)
		env.AssertPop(t, firstRun(env.UIDs())...)
	})

	t.Run("cluster_check_runner_skips_the_election", func(t *testing.T) {
		env := suite.Setup(t, suite.WithMode(suite.ClusterCheckRunner))
		env.SetLeader(false)

		env.AssertPop(t, firstRun(env.UIDs())...)
	})

	t.Run("leader_election_error_fails_the_run", func(t *testing.T) {
		env := suite.Setup(t)
		env.FailLeaderElection(errors.New("the lease is unavailable"))

		actual, err := env.PopWithError(t)
		assert.ErrorContains(t, err, "the lease is unavailable")
		suite.AssertObjects(t, nil, actual)
		env.FailLeaderElection(nil)
		env.AssertPop(t, firstRun(env.UIDs())...)
	})

	t.Run("cluster_agent_without_leader_election_fails", func(t *testing.T) {
		env := suite.Setup(t, suite.WithSetting("leader_election", false))

		actual, err := env.PopWithError(t)
		assert.ErrorContains(t, err, "Leader Election not enabled")
		suite.AssertObjects(t, nil, actual)
	})
}
