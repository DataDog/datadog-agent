// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"errors"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PatchMutator plans explicit edits in a shared request-local session.
// The bool retains the historical injection/metrics meaning, independently of
// whether annotation or normalization operations were recorded.
type PatchMutator interface {
	PlanPod(*patch.PodSession, string, dynamic.Interface) (bool, error)
}

// PatchMutatorFunc adapts an explicit mutation function.
type PatchMutatorFunc func(*patch.PodSession, string, dynamic.Interface) (bool, error)

// PlanPod implements PatchMutator.
func (f PatchMutatorFunc) PlanPod(s *patch.PodSession, ns string, dc dynamic.Interface) (bool, error) {
	return f(s, ns, dc)
}

// PatchMutators composes decisions and edits in their existing order.
type PatchMutators []PatchMutator

// NewPatchMutators constructs an ordered composition using one session.
func NewPatchMutators(mutators ...PatchMutator) PatchMutators { return PatchMutators(mutators) }

// PlanPod applies each stage once; later stages observe prior successful edits.
func (m PatchMutators) PlanPod(s *patch.PodSession, ns string, dc dynamic.Interface) (bool, error) {
	injected := false
	for _, mutator := range m {
		changed, err := mutator.PlanPod(s, ns, dc)
		if err != nil {
			return injected, err
		}
		if err := s.Err(); err != nil {
			return injected, err
		}
		injected = injected || changed
	}
	return injected, nil
}

// MutateWithPatch returns the journal rather than diffing a typed Pod.
// Errors discard all request edits, including normalization.
func MutateWithPatch(raw []byte, ns, mutationType string, m PatchMutatorFunc, dc dynamic.Interface) ([]byte, error) {
	s, err := patch.NewPodSession(raw)
	if err != nil {
		metrics.MutationAttempts.Inc(mutationType, metrics.StatusError, "false", metrics.InvalidInput)
		return nil, err
	}
	if err := s.NormalizeVolumes(); err != nil {
		if !errors.Is(err, patch.ErrNormalizationConflict) {
			metrics.MutationAttempts.Inc(mutationType, metrics.StatusError, "false", metrics.InternalError)
			return nil, err
		}
		log.Warn("Cannot normalize differing duplicate volumes; API server may reject the Pod")
	}
	injected, err := m(s, ns, dc)
	if err != nil {
		label := err.Error()
		if s.Err() != nil {
			label = metrics.InternalError
		}
		metrics.MutationAttempts.Inc(mutationType, metrics.StatusError, "false", label)
		return nil, fmt.Errorf("failed to mutate pod: %w", err)
	}
	wire, err := s.JSONPatch()
	if err != nil {
		metrics.MutationAttempts.Inc(mutationType, metrics.StatusError, "false", metrics.InternalError)
		return nil, err
	}
	metrics.MutationAttempts.Inc(mutationType, metrics.StatusSuccess, strconv.FormatBool(injected), "")
	return wire, nil
}

// PatchInjectEnv preserves InjectEnv's no-overwrite and repeated-prepend behavior.
func PatchInjectEnv(s *patch.PodSession, env corev1.EnvVar) (bool, error) {
	return PatchInjectDynamicEnv(s, func(*corev1.Container, bool) (corev1.EnvVar, error) { return env, nil })
}

// PatchInjectDynamicEnv builds each feature value once and batches insertions.
// Existing best-effort value construction failures skip the selected container.
func PatchInjectDynamicEnv(s *patch.PodSession, fn BuildEnvVarFunc) (bool, error) {
	pod, err := s.Snapshot()
	if err != nil {
		return false, err
	}
	var injections []patch.EnvInjection
	for _, group := range []struct {
		kind       patch.ContainerKind
		containers []corev1.Container
	}{
		{patch.RegularContainers, pod.Spec.Containers}, {patch.InitContainers, pod.Spec.InitContainers},
	} {
		for _, container := range group.containers {
			env, err := fn(&container, group.kind == patch.InitContainers)
			if err != nil {
				log.Errorf("Error building env var: %v", err)
				continue
			}
			injections = append(injections, patch.EnvInjection{Container: patch.ContainerID{Kind: group.kind, Name: container.Name}, Env: env, Prepend: true})
		}
	}
	return s.EnsureEnvs(injections)
}

// PatchInjectVolume preserves the existing name-or-path mount collision policy.
func PatchInjectVolume(s *patch.PodSession, volume corev1.Volume, mount corev1.VolumeMount) (bool, bool, error) {
	pod, err := s.Snapshot()
	if err != nil {
		return false, false, err
	}
	var edits []patch.MountEdit
	for _, group := range []struct {
		kind       patch.ContainerKind
		containers []corev1.Container
	}{
		{patch.RegularContainers, pod.Spec.Containers}, {patch.InitContainers, pod.Spec.InitContainers},
	} {
		for _, container := range group.containers {
			if !containsVolumeMount(container.VolumeMounts, mount) {
				edits = append(edits, patch.MountEdit{Container: patch.ContainerID{Kind: group.kind, Name: container.Name}, Mount: mount})
			}
		}
	}
	if len(edits) == 0 {
		return false, false, nil
	}
	var fresh = &volume
	for _, existing := range pod.Spec.Volumes {
		if existing.Name == volume.Name {
			fresh = nil
			break
		}
	}
	if err := s.WriteVolumeMounts(fresh, edits); err != nil {
		return false, false, err
	}
	return fresh != nil, true, nil
}
