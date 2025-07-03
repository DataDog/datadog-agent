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

	// the next two sources are used to denote whether the service name
	// came from labels or annotations (when mapping pod meta as tags).
	serviceNameSourceLabelsAsTags      = "labels_as_tags"
	serviceNameSourceAnnotationsAsTags = "annotations_as_tags"
)

type serviceNameMutator struct {
	EnvVar corev1.EnvVar
	Source serviceNameSource
}

func (s *serviceNameMutator) mutateContainer(c *corev1.Container) error {
	if s == nil {
		return nil
	}

	var source *corev1.EnvVar
	if s.Source != "" {
		source = &corev1.EnvVar{Name: "DD_SERVICE_K8S_ENV_SOURCE"}
		if s.EnvVar.Value != "" {
			source.Value = fmt.Sprintf("%s=%s", s.Source, s.EnvVar.Value)
		} else {
			source.Value = string(s.Source)
		}
	}

	mutator := &ustEnvVarMutator{
		EnvVar: s.EnvVar,
		Source: source,
	}

	return mutator.mutateContainer(c)
}

func serviceNameMutatorForMetaAsTags(pod *corev1.Pod, t podMetaAsTags) *serviceNameMutator {
	for _, check := range []struct {
		kind   podMetaKind
		source serviceNameSource
	}{
		{podMetaKindLabels, serviceNameSourceLabelsAsTags},
		{podMetaKindAnnotations, serviceNameSourceAnnotationsAsTags},
	} {
		if env, found := envVarForPodMetaMapping(pod, check.kind, t, tags.Service, kubernetes.ServiceTagEnvVar); found {
			return &serviceNameMutator{EnvVar: env, Source: check.source}
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
	log.Debug("checking metaAsTags")
	if mutator := serviceNameMutatorForMetaAsTags(pod, t); mutator != nil {
		return mutator
	}

	log.Debug("no service env vars found & tags found in pod, checking owner name")
	name, err := getServiceNameFromPodOwnerName(pod)
	if err != nil || name == "" {
		log.Debugf("error getting owner name for pod: %v", err)
		return nil
	}

	if name == "" {
		return nil
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
