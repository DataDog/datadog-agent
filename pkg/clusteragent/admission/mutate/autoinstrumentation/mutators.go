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

func (mutators containerMutators) mutateContainer(c *corev1.Container) error {
	for _, m := range mutators {
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

func (v volume) mount(mount corev1.VolumeMount) volumeMount {
	mount.Name = v.Name
	return volumeMount{VolumeMount: mount}
}

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

func (v volumeMount) readOnly() volumeMount {
	m := v.VolumeMount
	m.ReadOnly = true
	return volumeMount{m, v.Prepend}
}

func (v volumeMount) prepended() volumeMount {
	v2 := v
	v2.Prepend = true
	return v2
}

func appendOrPrepend[T any](item T, toList []T, prepend bool) []T {
	if prepend {
		return append([]T{item}, toList...)
	}

	return append(toList, item)
}

type configKeyEnvVarMutator struct {
	envKey string
	envVal string
}

func newConfigEnvVarFromBoolMutator(key string, val *bool) configKeyEnvVarMutator {
	m := configKeyEnvVarMutator{
		envKey: key,
	}

	if val == nil {
		m.envVal = strconv.FormatBool(false)
	} else {
		m.envVal = strconv.FormatBool(*val)
	}

	return m
}

func newConfigEnvVarFromStringlMutator(key string, val *string) configKeyEnvVarMutator {
	m := configKeyEnvVarMutator{
		envKey: key,
	}

	if val != nil {
		m.envVal = *val
	}

	return m
}

func (c configKeyEnvVarMutator) mutatePod(pod *corev1.Pod) error {
	_ = common.InjectEnv(pod, corev1.EnvVar{Name: c.envKey, Value: c.envVal})

	return nil
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
