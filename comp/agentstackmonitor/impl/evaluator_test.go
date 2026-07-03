// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	issuetemplates "github.com/DataDog/datadog-agent/comp/healthplatform/issues/agentstackmonitor"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// baseState constructs a subjectState with identity fields set but no
// observations. Tests push signals via the ring helpers.
func baseState() *subjectState {
	return &subjectState{
		subjectKind:   SubjectKindClusterAgent,
		controller:    controllerRef{Namespace: "monitoring", Kind: "Deployment", Name: "datadog-cluster-agent"},
		containerName: "cluster-agent",
		podName:       "datadog-cluster-agent-7f9c8d5b4-x2j9k",
		podUID:        "abc-uid",
	}
}

// reportByName returns the report with the matching IssueName, or nil.
func reportByName(reports []runnerdef.IssueReport, name string) *runnerdef.IssueReport {
	for i := range reports {
		if reports[i].IssueName == name {
			return &reports[i]
		}
	}
	return nil
}

func TestEvaluate_MemoryPressure(t *testing.T) {
	t.Run("no fire below the sample threshold", func(t *testing.T) {
		// Fewer than memPressureMinSamples samples over threshold — even if
		// every sample so far is over, we haven't seen 8 yet.
		s := baseState()
		for _, v := range []float64{0.95, 0.95, 0.95, 0.95, 0.95} {
			s.memRatio.push(v)
		}
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameMemoryPressure))
	})

	t.Run("no fire when only some samples exceed threshold", func(t *testing.T) {
		s := baseState()
		for _, v := range []float64{0.8, 0.85, 0.95, 0.6, 0.5, 0.99, 0.7, 0.4, 0.3, 0.95} {
			s.memRatio.push(v)
		}
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameMemoryPressure))
	})

	t.Run("fires as soon as 8 samples exceed threshold (partial buffer)", func(t *testing.T) {
		// 8-of-8 = 100% duty cycle at minute 8, no need to wait for a
		// full 10-sample window.
		s := baseState()
		for _, v := range []float64{0.95, 0.95, 0.95, 0.95, 0.95, 0.95, 0.95, 0.95} {
			s.memRatio.push(v)
		}
		s.memUsage = 9.5e8
		s.memLimit = 1e9
		got := reportByName(evaluate(s), issuetemplates.IssueNameMemoryPressure)
		require.NotNil(t, got, "expected memory pressure issue on 8-of-8")
		assert.Equal(t, "8", got.Context[issuetemplates.CtxSamplesOverThresh])
		assert.Equal(t,
			"agentstackmonitor.memory-pressure:monitoring/Deployment/datadog-cluster-agent:cluster-agent",
			got.IssueID)
	})

	t.Run("fires on 8-of-10 with two breather samples below threshold", func(t *testing.T) {
		s := baseState()
		for _, v := range []float64{0.95, 0.95, 0.95, 0.95, 0.95, 0.95, 0.95, 0.95, 0.5, 0.4} {
			s.memRatio.push(v)
		}
		assert.NotNil(t, reportByName(evaluate(s), issuetemplates.IssueNameMemoryPressure))
	})
}

func TestEvaluate_ContainerRestart(t *testing.T) {
	t.Run("no fire below the delta-sum threshold", func(t *testing.T) {
		s := baseState()
		// Only 2 restarts observed so far — below the > 2 threshold.
		for _, d := range []int32{0, 1, 0, 1} {
			s.restartDeltas.push(d)
		}
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameContainerRestart))
	})

	t.Run("no fire on a single restart flake in an otherwise quiet window", func(t *testing.T) {
		s := baseState()
		for _, d := range []int32{0, 0, 0, 0, 0, 1, 0, 0, 0, 0} {
			s.restartDeltas.push(d)
		}
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameContainerRestart))
	})

	t.Run("fires as soon as delta sum exceeds threshold (partial buffer)", func(t *testing.T) {
		// 3 restarts in 3 ticks — the "sustained and increasing" pattern
		// should fire at tick 3, not wait for a full 10-tick window.
		s := baseState()
		for _, d := range []int32{1, 1, 1} {
			s.restartDeltas.push(d)
		}
		got := reportByName(evaluate(s), issuetemplates.IssueNameContainerRestart)
		require.NotNil(t, got, "expected container restart issue at tick 3")
		assert.Equal(t, "3", got.Context[issuetemplates.CtxRestartsInWindow])
	})

	t.Run("fires when total deltas across a full window exceed threshold", func(t *testing.T) {
		s := baseState()
		for _, d := range []int32{0, 1, 0, 1, 0, 1, 0, 1, 0, 0} {
			s.restartDeltas.push(d)
		}
		got := reportByName(evaluate(s), issuetemplates.IssueNameContainerRestart)
		require.NotNil(t, got)
		assert.Equal(t, "4", got.Context[issuetemplates.CtxRestartsInWindow])
	})
}

func TestEvaluate_OOMKilled(t *testing.T) {
	t.Run("does not fire on stale LastTerminationState without a restart", func(t *testing.T) {
		s := baseState()
		for i := 0; i < bufferSize; i++ {
			s.restartDeltas.push(0)
		}
		s.lastTermReason = "OOMKilled"
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameContainerOOMKilled))
	})

	t.Run("fires when OOMKilled coincides with a restart in the window", func(t *testing.T) {
		s := baseState()
		for _, d := range []int32{0, 0, 0, 1, 0, 0, 0, 0, 0, 0} {
			s.restartDeltas.push(d)
		}
		s.lastTermReason = "OOMKilled"
		s.memLimit = 5.12e8
		got := reportByName(evaluate(s), issuetemplates.IssueNameContainerOOMKilled)
		assert.NotNil(t, got)
	})

	t.Run("does not fire when reason is not OOMKilled", func(t *testing.T) {
		s := baseState()
		s.restartDeltas.push(1)
		s.lastTermReason = "Error"
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameContainerOOMKilled))
	})
}

func TestEvaluate_CrashLoopBackOff(t *testing.T) {
	t.Run("no fire on a single observation", func(t *testing.T) {
		s := baseState()
		for _, r := range []string{"", "", "", "", "", "CrashLoopBackOff", "", "", "", ""} {
			s.waitingReason.push(r)
		}
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameCrashLoopBackOff))
	})

	t.Run("fires after 3 or more observations", func(t *testing.T) {
		s := baseState()
		for _, r := range []string{"", "", "CrashLoopBackOff", "CrashLoopBackOff", "", "", "CrashLoopBackOff", "", "", ""} {
			s.waitingReason.push(r)
		}
		got := reportByName(evaluate(s), issuetemplates.IssueNameCrashLoopBackOff)
		require.NotNil(t, got)
		assert.Equal(t, "3", got.Context[issuetemplates.CtxWaitingObservedIn])
	})

	t.Run("ignores other waiting reasons", func(t *testing.T) {
		s := baseState()
		for _, r := range []string{"", "ImagePullBackOff", "ImagePullBackOff", "ImagePullBackOff", "", "", "", "", "", ""} {
			s.waitingReason.push(r)
		}
		assert.Nil(t, reportByName(evaluate(s), issuetemplates.IssueNameCrashLoopBackOff))
	})
}

func TestEvaluate_NilState(t *testing.T) {
	assert.Nil(t, evaluate(nil))
}
