// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

// Package cases registers the tests of the orchestrator check suite, one group for each area.
package cases

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func init() { suite.Register("Lifecycle", lifecycle) }

// lifecycle lists, updates and deletes every object, in each deployment mode.
func lifecycle(t *testing.T, s *suite.Suite) {
	s.RunTests(t,
		// The check sends every object when it first runs, and again when it changes or is deleted.
		suite.Test{
			Name:  "Leader",
			Steps: everyObjectSteps,
		},
		// A follower sends nothing, whatever changes in the cluster.
		suite.Test{
			Name:  "Follower",
			Setup: suite.Setup{Mode: suite.ClusterAgentFollower},
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.NoObjects,
				},
				{
					Name:   "update every object",
					Change: suite.UpdateAll(suite.Collected),
					Expect: suite.NoObjects,
				},
				{
					Name:   "delete every object",
					Change: suite.DeleteAll(suite.Collected),
					Expect: suite.NoObjects,
				},
			},
		},
		suite.Test{
			Name:  "ClusterCheckRunner",
			Setup: suite.Setup{Mode: suite.ClusterCheckRunner},
			Steps: everyObjectSteps,
		},
		// The pods collector only lists unassigned pods, so the check knows an assigned pod from its deletion only.
		suite.Test{
			Name: "AssignedPodDeletion",
			Steps: []suite.Step{
				{
					Name:   "first run",
					Expect: suite.Live(suite.Collected),
				},
				{
					Name: "delete an assigned pod",
					Change: func(t *testing.T, env *suite.Env) {
						env.Cluster.Add(t, assignedPod.DeepCopy())
						env.Delete(t, assignedPod.DeepCopy())
					},
					Expect: func(t *testing.T, env *suite.Env) suite.Payloads {
						return env.PayloadsOf(t, "v1/terminated-pods", env.Deleted(t, string(assignedPod.UID)), true)
					},
					// The fake clients ignore the field selector of the unassigned pod informer, which can list the
					// assigned pod before its deletion.
					Ignore: func(s suite.Sent) bool { return s.UID == string(assignedPod.UID) && !s.Terminated },
				},
			},
		},
	)
}

// everyObjectSteps list, update and delete every object, which a leader and a cluster check runner send.
var everyObjectSteps = []suite.Step{
	{
		Name:   "first run",
		Expect: suite.Live(suite.Collected),
	},
	{
		Name:   "unchanged run",
		Expect: suite.NoObjects,
	},
	{
		Name:   "update every object",
		Change: suite.UpdateAll(suite.Collected),
		Expect: suite.Live(suite.Collected),
	},
	{
		Name:   "delete every object",
		Change: suite.DeleteAll(suite.Collected),
		Expect: suite.Deleted(suite.Collected),
	},
}

var assignedPod = &corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{Name: "assigned", Namespace: "default", UID: "assigned-pod", ResourceVersion: "1"},
	Spec:       corev1.PodSpec{NodeName: "node"},
}
