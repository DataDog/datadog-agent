// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clusterinfo"
)

// These tests drive GetStaticTagsSlice, as Agent startup does, with the real Cluster Agent
// client against a Cluster Agent that is up, down, wedged (accepts and never answers) or
// blackholed (connections never complete), and assert the wall-clock time startup waits. The
// budget is shortened: testBudget stands for the 10s StartupRetryBudget. TestStartupProbe measures the same paths with the production budgets.
const (
	testBudget = 2 * time.Second
	// slack covers scheduling and the local TLS handshake.
	slack = 400 * time.Millisecond
)

func setupStartupTest(t *testing.T, onEKS bool) {
	t.Helper()
	clusteragent.ResetGlobalClusterAgentClientForTest()
	t.Cleanup(clusteragent.ResetGlobalClusterAgentClientForTest)
	// The fake Cluster Agent's cluster ID and distribution land in the process-wide cache.
	cache.Cache.Flush()
	t.Cleanup(cache.Cache.Flush)
	clusterinfo.SetupStartupForTest(t, testBudget, onEKS)
}

func timedStaticTags(t *testing.T, state string, onEKS, fargate bool) (time.Duration, bool) {
	t.Helper()
	cfg := setupStartupNode(t, fakeClusterAgent(t, state, onEKS), fargate)
	setupStartupTest(t, onEKS)
	start := time.Now()
	tags := GetStaticTagsSlice(context.TODO(), cfg)
	elapsed := time.Since(start)
	for _, tag := range tags {
		if strings.HasPrefix(tag, "eks_cluster_arn:") {
			return elapsed, true
		}
	}
	return elapsed, false
}

var allClusterAgentStates = []string{dcaUp, dcaDown, dcaWedged, dcaBlackholed}

func TestStartupNonEKSNodeDoesNotWaitForTheClusterAgent(t *testing.T) {
	// Without local EKS evidence a node makes no Cluster Agent call at startup, as on main,
	// whatever state the Cluster Agent is in.
	for _, state := range allClusterAgentStates {
		t.Run(state, func(t *testing.T) {
			elapsed, identity := timedStaticTags(t, state, false, false)
			assert.Less(t, elapsed, 100*time.Millisecond)
			assert.False(t, identity)
		})
	}
}

func TestStartupEKSNodeWaitsAtMostTheBudget(t *testing.T) {
	for _, state := range allClusterAgentStates {
		t.Run(state, func(t *testing.T) {
			elapsed, identity := timedStaticTags(t, state, true, false)
			assert.Less(t, elapsed, testBudget+slack)
			assert.Equal(t, state == dcaUp, identity)
		})
	}
}

func TestStartupEKSNodeLaterCallersDoNotWaitAgain(t *testing.T) {
	// The tagger spends the budget against a wedged Cluster Agent. DogStatsD and OTLP right
	// after get the same failure at once.
	for _, state := range []string{dcaWedged, dcaBlackholed} {
		t.Run(state, func(t *testing.T) {
			cfg := setupStartupNode(t, fakeClusterAgent(t, state, true), false)
			setupStartupTest(t, true)
			GetStaticTagsSlice(context.TODO(), cfg)
			for range 2 {
				start := time.Now()
				GetStaticTagsSlice(context.TODO(), cfg)
				assert.Less(t, time.Since(start), 100*time.Millisecond)
			}
		})
	}
}

func TestStartupFargateSidecarWaitsAtMostOneBudget(t *testing.T) {
	// The cluster ID call and the identity call run at the same time, each bounded to the
	// startup budget: startup waits once, and a Cluster Agent that hangs on the cluster ID does
	// not starve the identity call.
	for _, state := range append(allClusterAgentStates, dcaWedgedClusterID) {
		t.Run(state, func(t *testing.T) {
			elapsed, identity := timedStaticTags(t, state, true, true)
			assert.Less(t, elapsed, testBudget+slack)
			assert.Equal(t, state == dcaUp || state == dcaWedgedClusterID, identity)
		})
	}
}

func TestStartupFargateSidecarBudgetWithoutCallerDeadline(t *testing.T) {
	// The caller passes a ctx without a deadline, as DogStatsD does. Each call must bound itself
	// to the startup budget: a call that waited on the caller's ctx would never return.
	cfg := setupStartupNode(t, fakeClusterAgent(t, dcaWedged, true), true)
	setupStartupTest(t, true)
	start := time.Now()
	GetStaticTagsSlice(context.Background(), cfg)
	assert.Less(t, time.Since(start), testBudget+slack)
}
