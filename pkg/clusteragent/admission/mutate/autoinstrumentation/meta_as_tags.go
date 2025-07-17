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
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type podMetaKind string

const (
	podMetaKindLabels      podMetaKind = "labels"
	podMetaKindAnnotations podMetaKind = "annotations"
)

type tagEnvVar struct {
	tag        string
	envVarName string
}

type ustEnvVarSourceKind string

const (
	ustEnvVarSourceKindPodLabels            ustEnvVarSourceKind = "labels_as_tags"
	ustEnvVarSourceKindPodAnnotations       ustEnvVarSourceKind = "annotations_as_tags"
	ustEnvVarSourceKindNamespaceLabels      ustEnvVarSourceKind = "namespace_labels_as_tags"
	ustEnvVarSourceKindNamespaceAnnotations ustEnvVarSourceKind = "namespace_annotations_as_tags"
)

type resourceMetaAsTags struct {
	Labels      map[string]string
	Annotations map[string]string
}

type metaAsTags struct {
	Pod       resourceMetaAsTags
	Namespace resourceMetaAsTags
}

var (
	serviceUST = tagEnvVar{tag: tags.Service, envVarName: kubernetes.ServiceTagEnvVar}
	envUST     = tagEnvVar{tag: tags.Env, envVarName: kubernetes.EnvTagEnvVar}
	versionUST = tagEnvVar{tag: tags.Version, envVarName: kubernetes.VersionTagEnvVar}
	ustsByTag  = map[string]tagEnvVar{
		tags.Service: serviceUST,
		tags.Env:     envUST,
		tags.Version: versionUST,
	}
)

func getMetaAsTags(datadogConfig config.Component) metaAsTags {
	tags := configUtils.GetMetadataAsTags(datadogConfig)
	return metaAsTags{
		Pod: resourceMetaAsTags{
			Labels:      tags.GetPodLabelsAsTags(),
			Annotations: tags.GetPodAnnotationsAsTags(),
		},
		Namespace: resourceMetaAsTags{
			Labels:      tags.GetNamespaceLabelsAsTags(),
			Annotations: tags.GetNamespaceAnnotationsAsTags(),
		},
	}
}

func (t tagEnvVar) envVarForMetaMapping(
	mapping, meta map[string]string,
	setValue func(env *corev1.EnvVar, key, value string),
) (corev1.EnvVar, bool) {
	for key, target := range mapping {
		if target != t.tag {
			continue
		}

		value, exists := meta[key]
		if !exists {
			continue
		}

		env := corev1.EnvVar{Name: t.envVarName}
		setValue(&env, key, value)
		return env, true
	}

	return corev1.EnvVar{}, false
}

// envVarSourceFromFieldRef is a helper function to set an EnvVarSource
// for a given kind and path, e.g. "metadata.annotations['app.kubernetes.io/name']"
func envVarSourceFromFieldRef(kind podMetaKind) func(env *corev1.EnvVar, key, _ string) {
	return func(env *corev1.EnvVar, key, _ string) {
		env.ValueFrom = &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: fmt.Sprintf("metadata.%s['%s']", kind, key),
			},
		}
	}
}

func directlySetEnvValue(env *corev1.EnvVar, _ string, value string) {
	env.Value = value
}

func (t tagEnvVar) ustEnvVarMutatorForPodMeta(
	wmeta workloadmeta.Component, pod *corev1.Pod,
	source metaAsTags,
) *ustEnvVarMutator {
	// look for labels
	if env, found := t.envVarForMetaMapping(source.Pod.Labels, pod.Labels, envVarSourceFromFieldRef(podMetaKindLabels)); found {
		return &ustEnvVarMutator{EnvVar: env, SourceKind: ustEnvVarSourceKindPodLabels}
	}

	// look for annotations
	if env, found := t.envVarForMetaMapping(source.Pod.Annotations, pod.Annotations, envVarSourceFromFieldRef(podMetaKindAnnotations)); found {
		return &ustEnvVarMutator{EnvVar: env, SourceKind: ustEnvVarSourceKindPodAnnotations}
	}

	// look for namespace metadata
	if len(source.Namespace.Labels) > 0 || len(source.Namespace.Annotations) > 0 {
		ns, err := wmetaNamespace(wmeta, pod.Namespace)
		if err != nil {
			log.Errorf("error getting namespace %s: %s", pod.Namespace, err)
			return nil
		}

		if env, found := t.envVarForMetaMapping(source.Namespace.Labels, ns.Labels, directlySetEnvValue); found {
			return &ustEnvVarMutator{EnvVar: env, SourceKind: ustEnvVarSourceKindNamespaceLabels}
		}

		if env, found := t.envVarForMetaMapping(source.Namespace.Annotations, ns.Annotations, directlySetEnvValue); found {
			return &ustEnvVarMutator{EnvVar: env, SourceKind: ustEnvVarSourceKindNamespaceAnnotations}
		}
	}

	return nil
}

// ustEnvVarMutator is a container mutator that adds
// a specific env var to the container (prepending it).
//
// These are constructed by using the [[ustEnvVarMutatorForPodMeta]] function.
type ustEnvVarMutator struct {
	EnvVar     corev1.EnvVar
	Source     *corev1.EnvVar
	SourceKind ustEnvVarSourceKind
}

// mutateContainer fulfills the [[containerMutator]] interface.
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
