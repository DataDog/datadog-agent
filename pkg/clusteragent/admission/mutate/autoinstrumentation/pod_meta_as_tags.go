// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type podMetaKind string

const (
	podMetaKindLabels      podMetaKind = "labels"
	podMetaKindAnnotations podMetaKind = "annotations"
)

type podMetaAsTags struct {
	Labels      map[string]string
	Annotations map[string]string
}

func getPodMetaAsTags(datadogConfig config.Component) podMetaAsTags {
	tags := configUtils.GetMetadataAsTags(datadogConfig)
	return podMetaAsTags{
		Labels:      tags.GetPodLabelsAsTags(),
		Annotations: tags.GetPodAnnotationsAsTags(),
	}
}

func envVarForPodMetaMapping(pod *corev1.Pod, kind podMetaKind, mappingSource podMetaAsTags, tag, envVarName string) (corev1.EnvVar, bool) {
	var mapping, podMeta map[string]string
	switch kind {
	case podMetaKindLabels:
		mapping = mappingSource.Labels
		podMeta = pod.Labels
	case podMetaKindAnnotations:
		mapping = mappingSource.Annotations
		podMeta = pod.Annotations
	default:
		log.Errorf("unexpected pod meta kind: %s", kind)
		return corev1.EnvVar{}, false
	}

	// Check if any of the mapping keys exist in the pod metadata
	for key, value := range mapping {
		if value != tag {
			continue
		}

		_, exists := podMeta[key]
		if !exists {
			continue
		}

		return corev1.EnvVar{
			Name:      envVarName,
			ValueFrom: envVarSourceFromFieldRef(kind, key),
		}, true
	}

	return corev1.EnvVar{}, false
}

// envVarSourceFromFieldRef is a helper function to create an EnvVarSource
// for a given kind and path, e.g. "metadata.annotations['app.kubernetes.io/name']"
func envVarSourceFromFieldRef(kind podMetaKind, path string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			FieldPath: fmt.Sprintf("metadata.%s['%s']", kind, path),
		},
	}
}

func ustEnvVarMutatorForPodMeta(pod *corev1.Pod, mappingSource podMetaAsTags, tag, envVarName string) *ustEnvVarMutator {
	env, found := envVarForPodMetaMapping(pod, podMetaKindLabels, mappingSource, tag, envVarName)
	if found {
		return &ustEnvVarMutator{EnvVar: env}
	}

	if env, found := envVarForPodMetaMapping(pod, podMetaKindAnnotations, mappingSource, tag, envVarName); found {
		return &ustEnvVarMutator{EnvVar: env}
	}

	return nil
}

type ustEnvVarMutator struct {
	EnvVar corev1.EnvVar
	Source *corev1.EnvVar
}

func (m *ustEnvVarMutator) mutateContainer(c *corev1.Container) error {
	if m == nil {
		return nil
	}

	if m.EnvVar.Name == "" {
		log.Errorf("env var name is empty, skipping mutator")
		return nil
	}

	for _, e := range c.Env {
		if e.Name == m.EnvVar.Name {
			return nil
		}
	}

	var envs []corev1.EnvVar
	envs = append(envs, m.EnvVar)

	if m.Source != nil {
		envs = append(envs, *m.Source)
	}

	c.Env = append(envs, c.Env...)
	return nil
}
