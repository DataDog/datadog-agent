// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"encoding/json"
	"errors"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func assertJournal(t *testing.T, s *PodSession) []byte {
	t.Helper()
	wire, err := s.JSONPatch()
	require.NoError(t, err)
	p, err := jsonpatch.DecodePatch(wire)
	require.NoError(t, err)
	options := jsonpatch.NewApplyOptions()
	options.SupportNegativeIndices = false
	options.AllowMissingPathOnRemove = false
	options.EnsurePathExistsOnAdd = false
	out, err := p.ApplyWithOptions(s.original, options)
	require.NoError(t, err)
	require.True(t, exactEqual(out, s.current), "returned journal must reproduce the decision document")
	return out
}

func TestParentsAndNumbers(t *testing.T) {
	for _, resources := range []string{"", `,"resources":null`, `,"resources":{"custom":{"n":18446744073709551617,"d":1.123456789012345678901}}`} {
		t.Run(resources, func(t *testing.T) {
			raw := []byte(`{"metadata":{"annotations":null},"spec":{"containers":[{"name":"app"` + resources + `}]},"extension":18446744073709551617}`)
			s, err := NewPodSession(raw)
			require.NoError(t, err)
			quantity := resource.MustParse("100m")
			require.NoError(t, s.EditResources([]ResourceEdit{{Container: ContainerID{RegularContainers, "app"}, Name: corev1.ResourceCPU, Quantity: &quantity}}))
			require.NoError(t, s.SetAnnotations(map[string]string{"a/b~c": "yes", "other": "value"}, false))
			out := assertJournal(t, s)
			require.Contains(t, string(out), `18446744073709551617`)
			if resources != "" && resources != `,"resources":null` {
				require.Contains(t, string(out), `1.123456789012345678901`)
			}
			wire, err := s.JSONPatch()
			require.NoError(t, err)
			require.Contains(t, string(wire), "/metadata/annotations/a~1b~0c")
			pod, err := s.Snapshot()
			require.NoError(t, err)
			require.Equal(t, "yes", pod.Annotations["a/b~c"])
		})
	}
}

func TestSnapshotsAndOccurrenceIdentity(t *testing.T) {
	raw := []byte(`{"spec":{"containers":[{"name":"app","env":[{"name":"X","value":"one","extension":2},{"name":"Y","value":"$(X)"},{"name":"X","value":"two"}]}]}}`)
	s, err := NewPodSession(raw)
	require.NoError(t, err)
	id := ContainerID{RegularContainers, "app"}
	pod, err := s.Snapshot()
	require.NoError(t, err)
	pod.Spec.Containers[0].Name = "detached"
	matches, err := s.FindEnv(id, "X")
	require.NoError(t, err)
	changed, err := s.EnsureEnvs([]EnvInjection{{id, corev1.EnvVar{Name: "DD_TEST", Value: "yes"}, true}})
	require.NoError(t, err)
	require.True(t, changed)
	out := assertJournal(t, s)
	var result corev1.Pod
	require.NoError(t, json.Unmarshal(out, &result))
	require.Equal(t, []corev1.EnvVar{{Name: "DD_TEST", Value: "yes"}, {Name: "X", Value: "one"}, {Name: "Y", Value: "$(X)"}, {Name: "X", Value: "two"}}, result.Spec.Containers[0].Env)
	newMatches, err := s.FindEnv(id, "X")
	require.NoError(t, err)
	require.Len(t, newMatches, 2)
	require.NoError(t, s.SetEnvOccurrence(newMatches[0], corev1.EnvVar{Name: "X", Value: "updated"}))
	out = assertJournal(t, s)
	require.Contains(t, string(out), `"extension":2`)
	require.ErrorContains(t, s.SetEnvOccurrence(matches[0], corev1.EnvVar{Name: "X", Value: "wrong"}), "stale")
	_, err = s.JSONPatch()
	require.Error(t, err)
}

func TestBatchFailureAndIgnoredErrors(t *testing.T) {
	s, err := NewPodSession([]byte(`{"spec":{"containers":[{"name":"app"}]}}`))
	require.NoError(t, err)
	original := string(s.current)
	err = s.batch(func(b *batch) error {
		if err := b.set([]string{"metadata", "annotations", "partial"}, "no"); err != nil {
			return err
		}
		b.ops = append(b.ops, operation{Op: "remove", Path: "/missing"})
		return nil
	})
	require.Error(t, err)
	require.Equal(t, original, string(s.current))
	require.False(t, s.HasOperations())
	_, err = s.JSONPatch()
	require.Error(t, err)

	s, err = NewPodSession([]byte(`{"extension":{"scalar":1}}`))
	require.NoError(t, err)
	_ = s.batch(func(b *batch) error { return b.set([]string{"extension", "scalar", "child"}, "no") })
	require.Error(t, s.Err())
	require.False(t, s.HasOperations())
}

func TestNormalization(t *testing.T) {
	for _, tc := range []struct {
		name       string
		volumes    string
		conflict   bool
		operations int
	}{
		{"exact raw duplicates", `[{"name":"v","custom":{"n":18446744073709551617}},{"custom":{"n":18446744073709551617},"name":"v"},{"name":"v","custom":{"n":18446744073709551617}}]`, false, 2},
		{"extension conflict", `[{"name":"v","custom":1},{"name":"v","custom":2}]`, true, 0},
		{"number spelling conflict", `[{"name":"v","custom":1},{"name":"v","custom":1.0}]`, true, 0},
		{"no partial normalization", `[{"name":"v"},{"name":"v"},{"name":"other","custom":1},{"name":"other","custom":2}]`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewPodSession([]byte(`{"spec":{"volumes":` + tc.volumes + `}}`))
			require.NoError(t, err)
			err = s.NormalizeVolumes()
			if tc.conflict {
				require.ErrorIs(t, err, ErrNormalizationConflict)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, s.Err())
			require.Len(t, s.journal, tc.operations)
			assertJournal(t, s)
		})
	}
}

func TestNoopAndDeletion(t *testing.T) {
	s, err := NewPodSession([]byte(`{"metadata":{"annotations":{"keep":"yes","remove":"no"}},"spec":{"containers":[{"name":"app","resources":{"requests":{"cpu":"0.1"},"custom":9}}],"volumes":[{"name":"a","custom":1},{"name":"keep","custom":2},{"name":"a","custom":3}]}}`))
	require.NoError(t, err)
	q := resource.MustParse("100m")
	require.NoError(t, s.EditResources([]ResourceEdit{{Container: ContainerID{RegularContainers, "app"}, Name: corev1.ResourceCPU, Quantity: &q}}))
	changed, err := s.EnsureEnvs(nil)
	require.NoError(t, err)
	require.False(t, changed)
	wire, err := s.JSONPatch()
	require.NoError(t, err)
	require.Equal(t, "[]", string(wire))
	require.NoError(t, s.RemoveAnnotations("remove", "absent"))
	require.NoError(t, s.EditResources([]ResourceEdit{{Container: ContainerID{RegularContainers, "app"}, Name: corev1.ResourceCPU}}))
	require.NoError(t, s.RemoveVolumes("a"))
	out := assertJournal(t, s)
	require.Contains(t, string(out), `"requests":{}`)
	require.Contains(t, string(out), `"custom":9`)
	require.Contains(t, string(out), `"name":"keep"`)
	require.NotContains(t, string(out), `"name":"a"`)
}

func TestAmbiguousContainerOnlyFailsWhenSelected(t *testing.T) {
	raw := []byte(`{"spec":{"containers":[{"name":"duplicate"},{"name":"duplicate"},{"name":"unique"}]}}`)
	s, err := NewPodSession(raw)
	require.NoError(t, err)
	require.NoError(t, s.SetAnnotations(map[string]string{"safe": "yes"}, false))
	_, err = s.EnsureEnvs([]EnvInjection{{ContainerID{RegularContainers, "unique"}, corev1.EnvVar{Name: "X", Value: "yes"}, true}})
	require.NoError(t, err)
	assertJournal(t, s)
	_, err = s.EnsureEnvs([]EnvInjection{{ContainerID{RegularContainers, "duplicate"}, corev1.EnvVar{Name: "X", Value: "no"}, true}})
	require.ErrorContains(t, err, "ambiguous")
}

func TestOwnedTemplatePreservesSurvivingObjects(t *testing.T) {
	raw := []byte(`{"spec":{"initContainers":[{"name":"injector","image":"old","extension":{"large":18446744073709551617},"env":[{"name":"A","valueFrom":{"secretKeyRef":{"name":"old","key":"old","custom":true}},"custom":"keep"}],"volumeMounts":[{"name":"v","mountPath":"/v","readOnly":true,"custom":"keep"}],"resources":{"custom":{"precise":0.12345678901234567890123456789},"requests":{"cpu":"50m"}},"securityContext":{"capabilities":{"add":["OLD"],"custom":true},"custom":true}}],"volumes":[{"name":"v","hostPath":{"path":"/old","custom":true},"custom":"keep"}]}}`)
	s, err := NewPodSession(raw)
	require.NoError(t, err)
	desired := corev1.Container{Name: "injector", Image: "new", Env: []corev1.EnvVar{{Name: "A", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "new"}, Key: "new"}}}}, VolumeMounts: []corev1.VolumeMount{{Name: "v", MountPath: "/v"}}, SecurityContext: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NEW"}}}}
	require.NoError(t, s.ConfigureInitContainerTemplate(desired, true))
	require.NoError(t, s.ReplaceVolumeSource("v", corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}))
	assertJournal(t, s)
	wire, err := s.JSONPatch()
	require.NoError(t, err)
	require.NotContains(t, string(wire), `"path":"/spec/initContainers/0"`)
	pod, err := s.Snapshot()
	require.NoError(t, err)
	require.Equal(t, "new", pod.Spec.InitContainers[0].Env[0].ValueFrom.SecretKeyRef.Name)
	require.False(t, pod.Spec.InitContainers[0].VolumeMounts[0].ReadOnly)
	require.Empty(t, pod.Spec.InitContainers[0].Resources.Requests)
	require.Contains(t, string(s.current), `18446744073709551617`)
	require.Contains(t, string(s.current), `0.12345678901234567890123456789`)
	require.Contains(t, string(s.current), `"custom":"keep"`)
	require.Contains(t, string(s.current), `"custom":true`)
	require.NotContains(t, string(s.current), `"hostPath"`)
	// Applying the same recipe again needs no operations.
	count := len(s.journal)
	require.NoError(t, s.ConfigureInitContainerTemplate(desired, true))
	require.Equal(t, count, len(s.journal))
}

func TestShiftedContainerCompositionAndIsolation(t *testing.T) {
	raw := []byte(`{"spec":{"containers":[{"name":"app","extension":18446744073709551617}]}}`)
	var expected []byte
	for range 2 {
		s, err := NewPodSession(raw)
		require.NoError(t, err)
		require.NoError(t, s.InsertContainer(RegularContainers, corev1.Container{Name: "sidecar"}, true))
		_, err = s.EnsureEnvs([]EnvInjection{{Container: ContainerID{RegularContainers, "app"}, Env: corev1.EnvVar{Name: "A", Value: "one"}}})
		require.NoError(t, err)
		require.NoError(t, s.SetAnnotations(map[string]string{"b": "two", "a": "one"}, false))
		assertJournal(t, s)
		wire, err := s.JSONPatch()
		require.NoError(t, err)
		require.Contains(t, string(wire), `/spec/containers/1/env`)
		c, err := s.ContainerSnapshot(ContainerID{RegularContainers, "app"})
		require.NoError(t, err)
		c.Env[0].Value = "detached"
		c, err = s.ContainerSnapshot(ContainerID{RegularContainers, "app"})
		require.NoError(t, err)
		require.Equal(t, "one", c.Env[0].Value)
		if expected == nil {
			expected = wire
		} else {
			require.Equal(t, expected, wire)
		}
	}
	require.Equal(t, `{"spec":{"containers":[{"name":"app","extension":18446744073709551617}]}}`, string(raw))
}

func TestContainerBatchReadAfterWriteAndFailure(t *testing.T) {
	raw := []byte(`{"spec":{"containers":[{"name":"app","args":["old"],"env":[{"name":"A","value":"old"}],"resources":{"custom":true},"volumeMounts":[{"name":"old","mountPath":"/old"}]}],"volumes":[{"name":"v"},{"name":"v"}]}}`)
	id := ContainerID{RegularContainers, "app"}
	s, err := NewPodSession(raw)
	require.NoError(t, err)
	require.NoError(t, s.ForContainers([]ContainerID{id}, func(id ContainerID) error {
		found, err := s.FindEnv(id, "A")
		require.NoError(t, err)
		require.NoError(t, s.SetEnvOccurrence(found[0], corev1.EnvVar{Name: "A", Value: "new"}))
		c, err := s.ContainerSnapshot(id)
		require.NoError(t, err)
		require.Equal(t, "new", c.Env[0].Value)
		require.NoError(t, s.RemoveVolumeMounts(id, "old"))
		require.NoError(t, s.ConfigureVolumeMount(id, corev1.VolumeMount{Name: "old", MountPath: "/new"}, false))
		require.NoError(t, s.SetMountPath(id, "old", "/current"))
		require.NoError(t, s.ConfigureVolumeMount(id, corev1.VolumeMount{Name: "old", MountPath: "/current", ReadOnly: true}, false))
		require.NoError(t, s.EditContainerArg(id, 0, "old", "new"))
		require.NoError(t, s.EditContainerArg(id, 0, "new", "final"))
		require.NoError(t, s.AppendContainerArg(id, "end"))
		quantity := resource.MustParse("100m")
		require.NoError(t, s.EditResources([]ResourceEdit{{Container: id, Name: corev1.ResourceCPU, Quantity: &quantity}}))
		require.NoError(t, s.ConfigureResources(id, corev1.ResourceRequirements{}))
		require.NoError(t, s.NormalizeVolumes())
		require.NoError(t, s.InsertContainer(RegularContainers, corev1.Container{Name: "sidecar"}, true))
		c, err = s.ContainerSnapshot(id)
		require.NoError(t, err)
		require.Equal(t, "new", c.Env[0].Value)
		require.Equal(t, []string{"final", "end"}, c.Args)
		require.Empty(t, c.Resources.Requests)
		require.Equal(t, []corev1.VolumeMount{{Name: "old", MountPath: "/current", ReadOnly: true}}, c.VolumeMounts)
		return nil
	}))
	assertJournal(t, s)
	// Even a whole-Pod snapshot inside the stage cannot publish partial failure.
	before := string(s.current)
	count := len(s.journal)
	err = s.ForContainers([]ContainerID{id}, func(id ContainerID) error {
		_, err := s.EnsureEnvs([]EnvInjection{{Container: id, Env: corev1.EnvVar{Name: "B", Value: "new"}, Prepend: true}})
		require.NoError(t, err)
		_, err = s.Snapshot()
		require.NoError(t, err)
		return errors.New("feature failed")
	})
	require.Error(t, err)
	require.Equal(t, before, string(s.current))
	require.Len(t, s.journal, count)
	// Handles also expire on pending writes inside a batch.
	err = s.ForContainers([]ContainerID{id}, func(id ContainerID) error {
		found, err := s.FindEnv(id, "A")
		require.NoError(t, err)
		_, err = s.EnsureEnvs([]EnvInjection{{Container: id, Env: corev1.EnvVar{Name: "B"}, Prepend: true}})
		require.NoError(t, err)
		return s.SetEnvOccurrence(found[0], corev1.EnvVar{Name: "A", Value: "wrong"})
	})
	require.ErrorContains(t, err, "stale")
	require.Equal(t, before, string(s.current))
	_, err = s.JSONPatch()
	require.Error(t, err)
}
