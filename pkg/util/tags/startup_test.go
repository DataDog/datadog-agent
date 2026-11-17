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
	// sharedConnBudget stands for the budget when a hung call holds the client's single
	// connection (MaxConnsPerHost 1): the other call waits for it up to the client's 3s
	// ResponseHeaderTimeout, so the budget must be above that, as the 10s production one is.
	sharedConnBudget = 4500 * time.Millisecond
	// slack covers scheduling and the local TLS handshake.
	slack = 400 * time.Millisecond
)

func setupStartupTest(t *testing.T, onEKS bool) {
	t.Helper()
	setupStartupTestWithBudget(t, onEKS, testBudget)
}

func setupStartupTestWithBudget(t *testing.T, onEKS bool, budget time.Duration) {
	t.Helper()
	clusteragent.ResetGlobalClusterAgentClientForTest()
	t.Cleanup(clusteragent.ResetGlobalClusterAgentClientForTest)
	// The fake Cluster Agent's cluster ID and distribution land in the process-wide cache.
	cache.Cache.Flush()
	t.Cleanup(cache.Cache.Flush)
	clusterinfo.SetupStartupForTest(t, budget, onEKS)
}

func timedStaticTags(t *testing.T, state string, onEKS, fargate bool) (time.Duration, bool) {
	t.Helper()
	return timedStaticTagsWithBudget(t, state, onEKS, fargate, testBudget)
}

func timedStaticTagsWithBudget(t *testing.T, state string, onEKS, fargate bool, budget time.Duration) (time.Duration, bool) {
	t.Helper()
	cfg := setupStartupNode(t, fakeClusterAgent(t, state, onEKS), fargate)
	setupStartupTestWithBudget(t, onEKS, budget)
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
	// The cluster ID call and the identity call run concurrently, each bounded to the startup
	// budget: startup waits once. They share the client's single connection, so a Cluster Agent
	// that hangs on the cluster ID delays the identity call by up to the client's 3s response
	// header timeout, but does not starve it.
	for _, state := range append(allClusterAgentStates, dcaWedgedClusterID) {
		t.Run(state, func(t *testing.T) {
			budget := testBudget
			if state == dcaWedgedClusterID {
				budget = sharedConnBudget
			}
			elapsed, identity := timedStaticTagsWithBudget(t, state, true, true, budget)
			assert.Less(t, elapsed, budget+slack)
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

func TestStartupRecoversFromProxyErrors(t *testing.T) {
	// A proxy in front of the Cluster Agent answers 502 for a while during a rollout. The node
	// and the sidecar retry within the budget and get the identity once it passes requests on.
	orig := proxyRecovery
	t.Cleanup(func() { proxyRecovery = orig })
	proxyRecovery = testBudget / 2
	for name, fargate := range map[string]bool{"node": false, "fargate": true} {
		t.Run(name, func(t *testing.T) {
			elapsed, identity := timedStaticTags(t, dcaProxy502, true, fargate)
			assert.True(t, identity)
			assert.GreaterOrEqual(t, elapsed, proxyRecovery)
			assert.Less(t, elapsed, testBudget+slack)
		})
	}
}
