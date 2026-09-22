// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDeploymentForReplicaSet(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001":  "frontend",
		"front-end-2891696001": "front-end",

		// Non-deployment 1.6 cases
		"frontend2891696001":  "",
		"-frontend2891696001": "",
		"manually-created":    "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7":   "frontend",
		"frontend-56c":          "frontend",
		"frontend-56c89cff":     "frontend",
		"frontend-56c89cfff7c2": "frontend",
		"front-end-768dd754b7":  "front-end",

		// 1.8+ non-deployment cases
		"frontend-5f":         "", // too short
		"frontend-56a89cfff7": "", // no vowels allowed
	} {
		t.Run("case: "+in, func(t *testing.T) {
			assert.Equal(t, out, ParseDeploymentForReplicaSet(in))
		})
	}
}

func TestParseDeploymentForPodName(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001-51234":  "frontend",
		"front-end-2891696001-72346": "front-end",

		// Non-deployment 1.6 cases
		"frontend2891696001-31-": "",
		"-frontend2891696001-21": "",
		"manually-created":       "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7-tsdww":   "frontend",
		"frontend-56c-p2q":            "frontend",
		"frontend-56c89cff-qhxl8":     "frontend",
		"frontend-56c89cfff7c2-g9lmb": "frontend",
		"front-end-768dd754b7-ptdcc":  "front-end",

		// 1.8+ non-deployment cases
		"frontend-56c89cff-bx":  "", // too short
		"frontend-56a89cfff7-a": "", // no vowels allowed
	} {
		t.Run("case: "+in, func(t *testing.T) {
			assert.Equal(t, out, ParseDeploymentForPodName(in))
		})
	}
}

func TestParseReplicaSetForPodName(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001-51234":  "frontend-2891696001",
		"front-end-2891696001-72346": "front-end-2891696001",

		// Non-replica-set 1.6 cases
		"frontend2891696001-31-": "",
		"-frontend2891696001-21": "",
		"manually-created":       "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7-tsdww":   "frontend-56c89cfff7",
		"frontend-56c-p2q":            "frontend-56c",
		"frontend-56c89cff-qhxl8":     "frontend-56c89cff",
		"frontend-56c89cfff7c2-g9lmb": "frontend-56c89cfff7c2",
		"front-end-768dd754b7-ptdcc":  "front-end-768dd754b7",

		// 1.8+ non-replica-set cases
		"frontend-56c89cff-bx":  "", // too short
		"frontend-56a89cfff7-a": "", // no vowels allowed
	} {
		t.Run("case: "+in, func(t *testing.T) {
			assert.Equal(t, out, ParseReplicaSetForPodName(in))
		})
	}
}

func TestParseCronJobForJob(t *testing.T) {
	for in, out := range map[string]struct {
		string
		int
	}{
		"hello-1562319360": {"hello", 1562319360},
		"hello-600":        {"hello", 600},
		"hello-world":      {"", 0},
		"hello":            {"", 0},
		"-hello1562319360": {"", 0},
		"hello1562319360":  {"", 0},
		"hello60":          {"", 0},
		"hello-60":         {"", 0},
		"hello-1562319a60": {"", 0},
	} {
		t.Run("case: "+in, func(t *testing.T) {
			cronjobName, id := ParseCronJobForJob(in)
			assert.Equal(t, out, struct {
				string
				int
			}{cronjobName, id})
		})
	}
}

func TestResolveWorkloadTarget(t *testing.T) {
	tests := []struct {
		name      string
		ownerKind string
		ownerName string
		podLabels map[string]string
		expected  WorkloadTarget
		expectOK  bool
	}{
		{
			name:      "deployment owner",
			ownerKind: DeploymentKind,
			ownerName: "my-app",
			expected:  WorkloadTarget{Kind: DeploymentKind, Namespace: "ns", Name: "my-app"},
			expectOK:  true,
		},
		{
			name:      "statefulset owner",
			ownerKind: StatefulSetKind,
			ownerName: "my-sts",
			expected:  WorkloadTarget{Kind: StatefulSetKind, Namespace: "ns", Name: "my-sts"},
			expectOK:  true,
		},
		{
			name:      "replicaset resolves to its deployment",
			ownerKind: ReplicaSetKind,
			ownerName: "my-app-7d9f8b6c5d",
			expected:  WorkloadTarget{Kind: DeploymentKind, Namespace: "ns", Name: "my-app"},
			expectOK:  true,
		},
		{
			// Argo Rollouts own pods through ReplicaSets too, so the parsed
			// parent is a Rollout rather than a Deployment. Getting this wrong
			// would attribute a same-named Deployment's autoscalers to it.
			name:      "replicaset of an argo rollout resolves to the rollout",
			ownerKind: ReplicaSetKind,
			ownerName: "my-app-7d9f8b6c5d",
			podLabels: map[string]string{ArgoRolloutLabelKey: "7d9f8b6c5d"},
			expected:  WorkloadTarget{Kind: RolloutKind, Namespace: "ns", Name: "my-app"},
			expectOK:  true,
		},
		{
			name:      "rollout owner",
			ownerKind: RolloutKind,
			ownerName: "my-rollout",
			expected:  WorkloadTarget{Kind: RolloutKind, Namespace: "ns", Name: "my-rollout"},
			expectOK:  true,
		},
		{
			name:      "replicaset without a parseable parent",
			ownerKind: ReplicaSetKind,
			ownerName: "noparent",
			expectOK:  false,
		},
		{
			name:      "unsupported owner kind",
			ownerKind: "DaemonSet",
			ownerName: "my-ds",
			expectOK:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, ok := ResolveWorkloadTarget("ns", test.ownerKind, test.ownerName, test.podLabels)
			assert.Equal(t, test.expectOK, ok)
			if test.expectOK {
				assert.Equal(t, test.expected, target)
			}
		})
	}
}
