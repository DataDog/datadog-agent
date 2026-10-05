// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
)

type containerMutator interface {
	planContainer(*patch.PodSession, patch.ContainerID) error
}
type containerMutatorFunc func(*patch.PodSession, patch.ContainerID) error

func (f containerMutatorFunc) planContainer(s *patch.PodSession, id patch.ContainerID) error {
	return f(s, id)
}

type containerMutators []containerMutator

func (ms containerMutators) planContainer(s *patch.PodSession, id patch.ContainerID) error {
	for _, m := range ms {
		if err := m.planContainer(s, id); err != nil {
			return err
		}
	}
	return nil
}

type podMutator interface{ planPod(*patch.PodSession) error }
type podMutatorFunc func(*patch.PodSession) error

func (f podMutatorFunc) planPod(s *patch.PodSession) error { return f(s) }
func snapshotContainer(s *patch.PodSession, id patch.ContainerID) (*corev1.Container, error) {
	return s.ContainerSnapshot(id)
}
func planPodContainers(s *patch.PodSession, m containerMutator, includeInit bool) error {
	pod, err := s.Snapshot()
	if err != nil {
		return err
	}
	var ids []patch.ContainerID
	if includeInit {
		for _, c := range pod.Spec.InitContainers {
			ids = append(ids, patch.ContainerID{Kind: patch.InitContainers, Name: c.Name})
		}
	}
	for _, c := range pod.Spec.Containers {
		ids = append(ids, patch.ContainerID{Kind: patch.RegularContainers, Name: c.Name})
	}
	return s.ForContainers(ids, func(id patch.ContainerID) error { return m.planContainer(s, id) })
}

type initContainer struct {
	corev1.Container
	Prepend  bool
	Mutators containerMutators
}

func (i initContainer) planPod(s *patch.PodSession) error {
	// This is a fresh recipe, so typed construction and serialization are safe.
	container := *i.Container.DeepCopy()
	if len(i.Mutators) > 0 {
		raw, err := json.Marshal(corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{container}}})
		if err != nil {
			return err
		}
		recipe, err := patch.NewPodSession(raw)
		if err != nil {
			return err
		}
		if err := i.Mutators.planContainer(recipe, patch.ContainerID{Kind: patch.RegularContainers, Name: container.Name}); err != nil {
			return err
		}
		pod, err := recipe.Snapshot()
		if err != nil {
			return err
		}
		container = pod.Spec.Containers[0]
	}
	return s.ConfigureInitContainerTemplate(container, i.Prepend)
}

type volumeMount struct {
	corev1.VolumeMount
	Prepend bool
}

func (v volumeMount) planContainer(s *patch.PodSession, id patch.ContainerID) error {
	return s.ConfigureVolumeMount(id, v.VolumeMount, v.Prepend)
}
func newConfigEnvVarFromBoolMutator(key string, val *bool) envVar {
	return envVarMutator(corev1.EnvVar{
		Name:  key,
		Value: strconv.FormatBool(valueOrZero(val)),
	})
}

func newConfigEnvVarFromStringMutator(key string, val *string) envVar {
	return envVarMutator(corev1.EnvVar{
		Name:  key,
		Value: valueOrZero(val),
	})
}

// containerFilter is a predicate function that evaluates
// a container and returns true or false.
//
// Used by filteredContainerMutator.
type containerFilter func(c *corev1.Container) bool

// filteredContainerMutator applies a containerFilter to the given
// containerMutator, producing a containerMutator.
func filteredContainerMutator(f containerFilter, m containerMutator) containerMutator {
	if f == nil {
		return m
	}
	return containerMutatorFunc(func(s *patch.PodSession, id patch.ContainerID) error {
		c, err := snapshotContainer(s, id)
		if err != nil {
			return err
		}
		if f != nil && !f(c) {
			return nil
		}
		return m.planContainer(s, id)
	})
}

// envVarMutator uses the envVar containerMutator to set the
// raw EnvVar as given.
//
// It will prepend the environment variable and if the variable already
// is in the container it will not add it.
//
// This is for parity for common.InjectEnv.
func envVarMutator(env corev1.EnvVar) envVar {
	return envVar{
		key:           env.Name,
		rawEnvVar:     &env,
		prepend:       true,
		dontOverwrite: true,
	}
}

type containerSecurityContext struct {
	*corev1.SecurityContext
}

func (r containerSecurityContext) planContainer(s *patch.PodSession, id patch.ContainerID) error {
	return s.ConfigureSecurityContext(id, r.SecurityContext)
}

type containerResourceRequirements struct {
	corev1.ResourceRequirements
}

func (r containerResourceRequirements) planContainer(s *patch.PodSession, id patch.ContainerID) error {
	return s.ConfigureResources(id, r.ResourceRequirements)
}
