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

func newServiceNameMutator(pod *corev1.Pod) *serviceNameMutator {
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

	log.Debug("no service env vars found in pod, checking owner name")
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
