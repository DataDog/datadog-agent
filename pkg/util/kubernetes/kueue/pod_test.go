// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestWorkloadName(t *testing.T) {
	for _, tt := range []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		want        string
	}{
		{name: "none"},
		{name: "admission annotation", annotations: map[string]string{kubernetes.KueueWorkloadAnnotationKey: "by-name"}, want: "by-name"},
		{name: "group label", labels: map[string]string{kubernetes.KueuePodGroupNameLabelKey: "by-name"}, want: "by-name"},
		{name: "group annotation", annotations: map[string]string{kubernetes.KueuePodGroupNameLabelKey: "by-name"}, want: "by-name"},
		{name: "prebuilt label", labels: map[string]string{kubernetes.KueuePrebuiltWorkloadNameKey: "by-name"}, want: "by-name"},
		{name: "prebuilt annotation", annotations: map[string]string{kubernetes.KueuePrebuiltWorkloadNameKey: "by-name"}, want: "by-name"},
		{name: "group annotation precedes label", annotations: map[string]string{kubernetes.KueuePodGroupNameLabelKey: "by-name"}, labels: map[string]string{kubernetes.KueuePodGroupNameLabelKey: "other"}, want: "by-name"},
		{name: "prebuilt annotation precedes label", annotations: map[string]string{kubernetes.KueuePrebuiltWorkloadNameKey: "by-name"}, labels: map[string]string{kubernetes.KueuePrebuiltWorkloadNameKey: "other"}, want: "by-name"},
		{name: "admission annotation precedes group", annotations: map[string]string{kubernetes.KueueWorkloadAnnotationKey: "by-name"}, labels: map[string]string{kubernetes.KueuePodGroupNameLabelKey: "other"}, want: "by-name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := &workloadmeta.KubernetesPod{EntityMeta: workloadmeta.EntityMeta{Labels: tt.labels, Annotations: tt.annotations}}
			assert.Equal(t, tt.want, WorkloadName(pod))
		})
	}
}

func TestIsManagedPod(t *testing.T) {
	assert.True(t, IsManagedPod(&workloadmeta.KubernetesPod{EntityMeta: workloadmeta.EntityMeta{Labels: map[string]string{kubernetes.KueueManagedLabelKey: "true"}}}))
	assert.False(t, IsManagedPod(&workloadmeta.KubernetesPod{EntityMeta: workloadmeta.EntityMeta{Labels: map[string]string{kubernetes.KueueManagedLabelKey: "false"}}}))
	assert.False(t, IsManagedPod(&workloadmeta.KubernetesPod{}))
}

func TestJobUID(t *testing.T) {
	assert.Equal(t, "pod-uid", JobUID(&workloadmeta.KubernetesKueueWorkload{EntityMeta: workloadmeta.EntityMeta{Labels: map[string]string{kubernetes.KueueJobUIDLabelKey: "pod-uid"}}}))
	assert.Empty(t, JobUID(&workloadmeta.KubernetesKueueWorkload{}))
}
