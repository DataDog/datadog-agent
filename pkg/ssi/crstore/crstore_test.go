// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package crstore

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newEntry(crName, crNamespace string) APMEntry {
	return APMEntry{
		CR:             types.NamespacedName{Namespace: crNamespace, Name: crName},
		Enabled:        true,
		TracerVersions: map[string]string{"java": "v1"},
		TracerConfigs:  []corev1.EnvVar{{Name: "DD_SERVICE", Value: "svc"}},
	}
}

func TestStore_UpsertAndGet(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
	entry := newEntry("ddi-web", "default")

	got, ok := s.GetAPM(workload)
	require.False(t, ok)
	assert.Equal(t, APMEntry{}, got)

	s.UpsertAPM(workload, entry)

	got, ok = s.GetAPM(workload)
	require.True(t, ok)
	assert.Equal(t, entry, got)
}

func TestStore_UpsertIsIdempotent(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
	entry := newEntry("ddi-web", "default")

	s.UpsertAPM(workload, entry)
	s.UpsertAPM(workload, entry)
	s.UpsertAPM(workload, entry)

	got, ok := s.GetAPM(workload)
	require.True(t, ok)
	assert.Equal(t, entry, got)
}

func TestStore_UpsertReplacesEntry(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}

	first := newEntry("ddi-web", "default")
	s.UpsertAPM(workload, first)

	second := first
	second.TracerVersions = map[string]string{"python": "v4"}
	s.UpsertAPM(workload, second)

	got, ok := s.GetAPM(workload)
	require.True(t, ok)
	assert.Equal(t, second, got)
}

func TestStore_CRRetargetedToDifferentWorkload(t *testing.T) {
	s := New()
	oldWorkload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web-old"}
	newWorkload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web-new"}
	entry := newEntry("ddi-web", "default")

	s.UpsertAPM(oldWorkload, entry)
	s.UpsertAPM(newWorkload, entry)

	_, ok := s.GetAPM(oldWorkload)
	assert.False(t, ok, "old workload entry should be cleared when CR is retargeted")

	got, ok := s.GetAPM(newWorkload)
	require.True(t, ok)
	assert.Equal(t, entry, got)
}

func TestStore_DeleteByCRClearsBothIndexes(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
	entry := newEntry("ddi-web", "default")

	s.UpsertAPM(workload, entry)
	s.DeleteByCR(entry.CR)

	_, ok := s.GetAPM(workload)
	assert.False(t, ok)

	// Repeat upsert should work cleanly after delete.
	s.UpsertAPM(workload, entry)
	_, ok = s.GetAPM(workload)
	assert.True(t, ok)
}

func TestStore_DeleteByCRIsNoOpForUnknown(_ *testing.T) {
	s := New()
	s.DeleteByCR(types.NamespacedName{Namespace: "default", Name: "missing"})
}

func TestStore_DeleteDoesNotEvictOtherCROnSameWorkload(t *testing.T) {
	// A second CR has taken over the workload before the first CR's delete is processed.
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}

	first := newEntry("ddi-first", "default")
	s.UpsertAPM(workload, first)

	// Simulate the second CR claiming the workload directly (without going through retarget).
	second := newEntry("ddi-second", "default")
	s.UpsertAPM(workload, second)

	// Delete the first CR — second's entry should be preserved.
	s.DeleteByCR(first.CR)

	got, ok := s.GetAPM(workload)
	require.True(t, ok)
	assert.Equal(t, second, got, "second CR's entry should remain")
}

func TestStore_ConcurrentAccess(_ *testing.T) {
	s := New()
	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
			entry := newEntry("ddi", "default")
			for j := range iterations {
				switch j % 3 {
				case 0:
					s.UpsertAPM(workload, entry)
				case 1:
					_, _ = s.GetAPM(workload)
				case 2:
					s.DeleteByCR(entry.CR)
				}
			}
		}(i)
	}
	wg.Wait()
}
