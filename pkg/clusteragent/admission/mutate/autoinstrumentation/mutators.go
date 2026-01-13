// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// containerMutator describes something that can mutate a container.
type containerMutator interface {
	mutateContainer(*corev1.Container) error
}

// containerMutatorFunc is a containerMutator as a function.
type containerMutatorFunc func(*corev1.Container) error

// mutateContainer implements containerMutator for containerMutatorFunc.
func (f containerMutatorFunc) mutateContainer(c *corev1.Container) error {
	return f(c)
}

type containerMutators []containerMutator

func (ms containerMutators) mutateContainer(c *corev1.Container) error {
	for _, m := range ms {
		if err := m.mutateContainer(c); err != nil {
			return err
		}
	}

	return nil
}

// podMutator describes something that can mutate a pod.
type podMutator interface {
	mutatePod(*corev1.Pod) error
}

// podMutatorFunc is a podMutator as a function.
type podMutatorFunc func(*corev1.Pod) error

// mutatePod implements podMutator.
func (f podMutatorFunc) mutatePod(pod *corev1.Pod) error {
	return f(pod)
}

// mutatePodContainers applies a containerMutator to containers of a pod.
// If includeInitContainers is true, it also applies to init containers.
func mutatePodContainers(pod *corev1.Pod, mutator containerMutator, includeInitContainers bool) error {
	if includeInitContainers {
		for idx, c := range pod.Spec.InitContainers {
			if err := mutator.mutateContainer(&c); err != nil {
				return err
			}
			pod.Spec.InitContainers[idx] = c
		}
	}

	for idx, c := range pod.Spec.Containers {
		if err := mutator.mutateContainer(&c); err != nil {
			return err
		}
		pod.Spec.Containers[idx] = c
	}

	return nil
}

// initContainer is a podMutator which adds the container to a pod as an
// init container. It will only add the container one time based on the
// container name.
//
// This has the option to both append and prepend the container to the list.
type initContainer struct {
	corev1.Container
	Prepend  bool
	Mutators containerMutators
}

var _ podMutator = (*initContainer)(nil)

// mutatePod implements podMutator for initContainer.
func (i initContainer) mutatePod(pod *corev1.Pod) error {
	container := i.Container

	if err := i.Mutators.mutateContainer(&container); err != nil {
		return err
	}

	for idx, c := range pod.Spec.InitContainers {
		if c.Name == container.Name {
			pod.Spec.InitContainers[idx] = container
			return nil
		}
	}

	pod.Spec.InitContainers = appendOrPrepend(container, pod.Spec.InitContainers, i.Prepend)
	return nil
}

// volume is a podMutator which adds the volume to a pod.
//
// It will only add the volume one time based on the volume name.
type volume struct {
	corev1.Volume
	Prepend bool
}

var _ podMutator = (*volume)(nil)

// mutatePod implements podMutator for volume.
func (v volume) mutatePod(pod *corev1.Pod) error {
	common.MarkVolumeAsSafeToEvictForAutoscaler(pod, v.Name)

	vol := v.Volume
	for idx, i := range pod.Spec.Volumes {
		if i.Name == v.Volume.Name {
			pod.Spec.Volumes[idx] = vol
			return nil
		}
	}

	pod.Spec.Volumes = appendOrPrepend(vol, pod.Spec.Volumes, v.Prepend)
	return nil
}

// volumeMount is a containerMutator which adds a volume mount to a container.
//
// It will only add the volumeMount one time based on Name and MountPath.
type volumeMount struct {
	corev1.VolumeMount
	Prepend bool
}

var _ containerMutator = (*volumeMount)(nil)

// mutateContainer implements containerMutator for volumeMount.
func (v volumeMount) mutateContainer(c *corev1.Container) error {
	mnt := v.VolumeMount
	for idx, vol := range c.VolumeMounts {
		if vol.Name == mnt.Name && vol.MountPath == mnt.MountPath {
			c.VolumeMounts[idx] = mnt
			return nil
		}
	}

	c.VolumeMounts = appendOrPrepend(mnt, c.VolumeMounts, v.Prepend)
	return nil
}

func (v volumeMount) readOnly() volumeMount { // nolint:unused
	m := v.VolumeMount
	m.ReadOnly = true
	return volumeMount{m, v.Prepend}
}

func appendOrPrepend[T any](item T, toList []T, prepend bool) []T {
	if prepend {
		return append([]T{item}, toList...)
	}

	return append(toList, item)
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
	return containerMutatorFunc(func(c *corev1.Container) error {
		if f != nil && !f(c) {
			return nil
		}
		return m.mutateContainer(c)
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

func (r containerSecurityContext) mutateContainer(c *corev1.Container) error {
	c.SecurityContext = r.SecurityContext
	return nil
}

type containerResourceRequirements struct {
	corev1.ResourceRequirements
}

func (r containerResourceRequirements) mutateContainer(c *corev1.Container) error {
	c.Resources = r.ResourceRequirements
	return nil
}
