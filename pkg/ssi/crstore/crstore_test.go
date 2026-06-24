// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package crstore

import (
	"sync"
	"testing"

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

func TestStoreUpsertAndGet(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
	entry := newEntry("ddi-web", "default")

	got, ok := s.GetAPM(workload)
	require.False(t, ok)
	require.Equal(t, APMEntry{}, got)

	s.UpsertAPM(workload, entry)

	got, ok = s.GetAPM(workload)
	require.True(t, ok)
	require.Equal(t, entry, got)
}

func TestStoreUpsertReplacesEntry(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}

	first := newEntry("ddi-web", "default")
	s.UpsertAPM(workload, first)

	second := first
	second.TracerVersions = map[string]string{"python": "v4"}
	s.UpsertAPM(workload, second)

	got, ok := s.GetAPM(workload)
	require.True(t, ok)
	require.Equal(t, second, got)
}

func TestStoreRetargetedCRClearsOldWorkload(t *testing.T) {
	s := New()
	oldWorkload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web-old"}
	newWorkload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web-new"}
	entry := newEntry("ddi-web", "default")

	s.UpsertAPM(oldWorkload, entry)
	s.UpsertAPM(newWorkload, entry)

	_, ok := s.GetAPM(oldWorkload)
	require.False(t, ok)

	got, ok := s.GetAPM(newWorkload)
	require.True(t, ok)
	require.Equal(t, entry, got)
}

func TestStoreDeleteByCR(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
	entry := newEntry("ddi-web", "default")

	s.UpsertAPM(workload, entry)
	s.DeleteByCR(entry.CR)

	_, ok := s.GetAPM(workload)
	require.False(t, ok)
}

func TestStoreDoesNotExposeMutableState(t *testing.T) {
	s := New()
	workload := WorkloadKey{Kind: "Deployment", Namespace: "default", Name: "web"}
	entry := newEntry("ddi-web", "default")
	s.UpsertAPM(workload, entry)

	got, ok := s.GetAPM(workload)
	require.True(t, ok)
	got.TracerVersions["java"] = "mutated"
	got.TracerConfigs[0].Value = "mutated"

	got, ok = s.GetAPM(workload)
	require.True(t, ok)
	require.Equal(t, "v1", got.TracerVersions["java"])
	require.Equal(t, "svc", got.TracerConfigs[0].Value)
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := New()
	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
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
		}()
	}
	wg.Wait()
}
