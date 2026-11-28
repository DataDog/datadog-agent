// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func sensitivePodTemplateSpec() v1.PodTemplateSpec {
	return v1.PodTemplateSpec{
		ObjectMeta: sensitiveObjectMeta(),
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{{Name: "init", Env: []v1.EnvVar{{Name: "API_KEY", Value: "sensitive-value"}}}},
			Containers: []v1.Container{{Name: "app", Env: []v1.EnvVar{
				{Name: "PASSWORD", Value: "sensitive-value"},
				{Name: "PORT", Value: "8080"},
			}}},
		},
	}
}

func assertScrubbedPodTemplateSpec(t *testing.T, template v1.PodTemplateSpec) {
	t.Helper()
	assertScrubbedObjectMeta(t, template.ObjectMeta)
	assert.Equal(t, []v1.EnvVar{{Name: "API_KEY", Value: redactedSecret}}, template.Spec.InitContainers[0].Env)
	assert.Equal(t, []v1.EnvVar{
		{Name: "PASSWORD", Value: redactedSecret},
		{Name: "PORT", Value: "8080"},
	}, template.Spec.Containers[0].Env)
}

func TestScrubWorkloads(t *testing.T) {
	scrubber := NewDefaultDataScrubber()

	deployment := &appsv1.Deployment{ObjectMeta: sensitiveObjectMeta(), Spec: appsv1.DeploymentSpec{Template: sensitivePodTemplateSpec()}}
	ScrubDeployment(deployment, scrubber)
	assertScrubbedObjectMeta(t, deployment.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, deployment.Spec.Template)

	daemonSet := &appsv1.DaemonSet{ObjectMeta: sensitiveObjectMeta(), Spec: appsv1.DaemonSetSpec{Template: sensitivePodTemplateSpec()}}
	ScrubDaemonSet(daemonSet, scrubber)
	assertScrubbedObjectMeta(t, daemonSet.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, daemonSet.Spec.Template)

	statefulSet := &appsv1.StatefulSet{ObjectMeta: sensitiveObjectMeta(), Spec: appsv1.StatefulSetSpec{Template: sensitivePodTemplateSpec()}}
	ScrubStatefulSet(statefulSet, scrubber)
	assertScrubbedObjectMeta(t, statefulSet.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, statefulSet.Spec.Template)

	replicaSet := &appsv1.ReplicaSet{ObjectMeta: sensitiveObjectMeta(), Spec: appsv1.ReplicaSetSpec{Template: sensitivePodTemplateSpec()}}
	ScrubReplicaSet(replicaSet, scrubber)
	assertScrubbedObjectMeta(t, replicaSet.ObjectMeta)
	assertScrubbedPodTemplateSpec(t, replicaSet.Spec.Template)
}

func TestScrubControllerRevision(t *testing.T) {
	for name, tt := range map[string]struct {
		raw, want string
	}{
		"object":  {raw: `{"spec":{"password":"sensitive-value","port":"8080"}}`, want: `{"spec":{"password":"********","port":"8080"}}`},
		"array":   {raw: `[{"token":"public"},{"secret":"sensitive-value"}]`, want: `[{"token":"public"},{"secret":"********"}]`},
		"invalid": {raw: `{"password":`, want: `"********"`},
		"empty":   {raw: "", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			revision := &appsv1.ControllerRevision{ObjectMeta: sensitiveObjectMeta(), Data: runtime.RawExtension{Raw: []byte(tt.raw)}}
			ScrubControllerRevision(revision, NewDefaultDataScrubber())
			assertScrubbedObjectMeta(t, revision.ObjectMeta)
			if tt.want == "" {
				assert.Empty(t, revision.Data.Raw)
			} else {
				assert.JSONEq(t, tt.want, string(revision.Data.Raw))
			}
		})
	}
}
