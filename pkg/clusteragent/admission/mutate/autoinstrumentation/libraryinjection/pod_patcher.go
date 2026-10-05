// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
)

// PodPatcher translates library injection intent into narrow session edits.
type PodPatcher struct {
	session *patch.PodSession
	filter  func(*corev1.Container) bool
}

// NewPodPatcher creates a writer using the shared request session.
func NewPodPatcher(session *patch.PodSession, filter func(*corev1.Container) bool) *PodPatcher {
	return &PodPatcher{session: session, filter: filter}
}

// AddVolume installs a source recipe, retaining unknown Volume siblings.
func (p *PodPatcher) AddVolume(volume corev1.Volume) error {
	pod, err := p.session.Snapshot()
	if err != nil {
		return err
	}
	found := false
	for _, existing := range pod.Spec.Volumes {
		if existing.Name == volume.Name {
			found = true
			break
		}
	}
	if found {
		err = p.session.ReplaceVolumeSource(volume.Name, volume.VolumeSource)
	} else {
		err = p.session.InsertVolume(volume, false)
	}
	if err != nil {
		return err
	}
	return p.session.MarkVolumeSafeToEvict(mutatecommon.K8sAutoscalerSafeToEvictVolumesAnnotation, volume.Name)
}

// AddVolumeMount uses the existing name+mountPath collision policy and prepend order.
func (p *PodPatcher) AddVolumeMount(mount corev1.VolumeMount) error {
	return p.AddVolumeMountWithTarget(mount, "")
}

// AddVolumeMountWithTarget applies a mount to selected application containers.
func (p *PodPatcher) AddVolumeMountWithTarget(mount corev1.VolumeMount, name string) error {
	pod, err := p.session.Snapshot()
	if err != nil {
		return err
	}
	var edits []patch.MountEdit
	for _, container := range pod.Spec.Containers {
		if p.filter != nil && !p.filter(&container) || name != "" && container.Name != name {
			continue
		}
		edits = append(edits, patch.MountEdit{Container: patch.ContainerID{Kind: patch.RegularContainers, Name: container.Name}, Mount: mount, Prepend: true, Configure: true})
	}
	return p.session.WriteVolumeMounts(nil, edits)
}

// AddInitContainer installs a fresh recipe or explicitly edits an existing one.
func (p *PodPatcher) AddInitContainer(container corev1.Container) error {
	return p.session.ConfigureInitContainerTemplate(container, true)
}

// AddEnvVar batches no-overwrite prepends into selected application containers.
func (p *PodPatcher) AddEnvVar(env corev1.EnvVar) error {
	pod, err := p.session.Snapshot()
	if err != nil {
		return err
	}
	var intents []patch.EnvInjection
	for _, container := range pod.Spec.Containers {
		if p.filter != nil && !p.filter(&container) {
			continue
		}
		intents = append(intents, patch.EnvInjection{Container: patch.ContainerID{Kind: patch.RegularContainers, Name: container.Name}, Env: env, Prepend: true})
	}
	_, err = p.session.EnsureEnvs(intents)
	return err
}

// AddEnvVarWithJoin joins the first existing occurrence or prepends a fresh entry.
func (p *PodPatcher) AddEnvVarWithJoin(name, value, separator string) error {
	pod, err := p.session.Snapshot()
	if err != nil {
		return err
	}
	var ids []patch.ContainerID
	for _, container := range pod.Spec.Containers {
		if p.filter == nil || p.filter(&container) {
			ids = append(ids, patch.ContainerID{Kind: patch.RegularContainers, Name: container.Name})
		}
	}
	return p.session.ForContainers(ids, func(id patch.ContainerID) error {
		container, err := p.session.ContainerSnapshot(id)
		if err != nil {
			return err
		}
		found := false
		for _, env := range container.Env {
			if env.Name != name {
				continue
			}
			found = true
			matches, err := p.session.FindEnv(id, name)
			if err != nil {
				return err
			}
			if err := p.session.SetEnvOccurrence(matches[0], corev1.EnvVar{Name: name, Value: env.Value + separator + value}); err != nil {
				return err
			}
			break
		}
		if !found {
			if _, err := p.session.EnsureEnvs([]patch.EnvInjection{{Container: id, Env: corev1.EnvVar{Name: name, Value: value}, Prepend: true}}); err != nil {
				return err
			}
		}
		return nil
	})
}
