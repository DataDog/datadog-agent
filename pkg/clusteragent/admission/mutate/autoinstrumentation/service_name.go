// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// serviceNameSource is used to denote where the service name
// is coming from when we inject it into a container using serviceNameMutator.
type serviceNameSource string

const (
	// serviceNameSourceExisting helps us know if we pulled the DD_SERVICE
	// from ane existing env var on the pod.
	serviceNameSourceExisting serviceNameSource = "existing"
	// serviceNameSourceOwnerName will tell us if we pulled the DD_SERVICE
	// from the pod owner name.
	serviceNameSourceOwnerName = "owner"

	serviceNameSourceLabelsAsTags      = "labels_as_tags"
	serviceNameSourceAnnotationsAsTags = "annotations_as_tags"
)

type serviceNameMutator struct {
	noop   bool
	EnvVar corev1.EnvVar     `json:"env"`
	Source serviceNameSource `json:"source"`
}

func (s *serviceNameMutator) mutateContainer(c *corev1.Container) error {
	if s.noop {
		return nil
	}

	for _, e := range c.Env {
		if e.Name == kubernetes.ServiceTagEnvVar {
			return nil
		}
	}

	var envs []corev1.EnvVar
	envs = append(envs, s.EnvVar)

	if s.Source != "" && s.EnvVar.Value != "" {
		envs = append(envs, corev1.EnvVar{
			Name:  "DD_SERVICE_K8S_ENV_SOURCE",
			Value: fmt.Sprintf("%s=%s", s.Source, s.EnvVar.Value),
		})
	}

	c.Env = append(envs, c.Env...)
	return nil
}

func doesMappedTagMatchValue(m map[string]string, k, v string) bool {
	if m != nil {
		if tag, matched := m[k]; matched {
			return tag == v
		}
	}
	return false
}

func serviceNameMutatorForMetaAsTags(
	podMeta map[string]string,
	match map[string]string,
	source serviceNameSource,
) *serviceNameMutator {
	for k, v := range podMeta {
		if !doesMappedTagMatchValue(match, k, tags.Service) {
			continue
		}

		var env corev1.EnvVar
		switch source {
		case serviceNameSourceAnnotationsAsTags:
			env.ValueFrom = &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fmt.Sprintf("metadata.annotations['%s']", k),
				},
			}
		case serviceNameSourceLabelsAsTags:
			env.ValueFrom = &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fmt.Sprintf("metadata.labels['%s']", k),
				},
			}
		default:
			log.Errorf("BUG: unexpected service name source %s", source)
			env.Value = v
		}

		return &serviceNameMutator{
			EnvVar: env,
			Source: source,
		}
	}

	return nil
}

func newServiceNameMutator(pod *corev1.Pod, t podMetaAsTags) *serviceNameMutator {
	vars := findServiceNameEnvVarsInPod(pod)
	if len(vars) > 1 {
		log.Debug("more than one unique definition of service name found for the pod")
	}

	if len(vars) > 0 {
		return &serviceNameMutator{
			EnvVar: vars[0],
			Source: serviceNameSourceExisting,
		}
	}

	log.Debug("no DD_SERVICE env vars found in pod")

	log.Debug("checking metaAsTags.labels")
	if mutator := serviceNameMutatorForMetaAsTags(pod.Labels, t.Labels, serviceNameSourceLabelsAsTags); mutator != nil {
		return mutator
	}

	log.Debug("checking metaAsTags.annotations")
	if mutator := serviceNameMutatorForMetaAsTags(pod.Annotations, t.Annotations, serviceNameSourceAnnotationsAsTags); mutator != nil {
		return mutator
	}

	log.Debug("no service env vars found & tags found in pod, checking owner name")
	name, err := getServiceNameFromPodOwnerName(pod)
	if err != nil || name == "" {
		log.Debugf("error getting owner name for pod: %v", err)
		return &serviceNameMutator{noop: true}
	}

	if name == "" {
		return &serviceNameMutator{noop: true}
	}

	return &serviceNameMutator{
		EnvVar: corev1.EnvVar{Name: kubernetes.ServiceTagEnvVar, Value: name},
		Source: serviceNameSourceOwnerName,
	}
}

func encodeEnvVar(e corev1.EnvVar) string {
	out, _ := json.Marshal(e)
	return string(out)
}

func findServiceNameEnvVarsInPod(pod *corev1.Pod) []corev1.EnvVar {
	found := []corev1.EnvVar{}
	keys := map[string]int{}

	iterContainer := func(c *corev1.Container) {
		for _, e := range c.Env {
			if e.Name == kubernetes.ServiceTagEnvVar {
				key := encodeEnvVar(e)
				_, ok := keys[key]
				if !ok {
					var env corev1.EnvVar
					e.DeepCopyInto(&env)
					found = append(found, env)
					idx := len(found) - 1
					keys[key] = idx
				}
				return
			}
		}
	}

	// we only look for the service name in the container (and not)
	// init containers.
	for _, c := range pod.Spec.Containers {
		iterContainer(&c)
	}

	return found
}

// Returns the name of Kubernetes resource that owns the pod
func getServiceNameFromPodOwnerName(pod *corev1.Pod) (string, error) {
	ownerReferences := pod.ObjectMeta.OwnerReferences
	if len(ownerReferences) != 1 {
		return "", fmt.Errorf("pod should be owned by one resource; current owners: %v+", ownerReferences)
	}

	switch owner := ownerReferences[0]; owner.Kind {
	case "StatefulSet":
		fallthrough
	case "Job":
		fallthrough
	case "CronJob":
		fallthrough
	case "DaemonSet":
		return owner.Name, nil
	case "ReplicaSet":
		return kubernetes.ParseDeploymentForReplicaSet(owner.Name), nil
	}

	return "", nil
}
