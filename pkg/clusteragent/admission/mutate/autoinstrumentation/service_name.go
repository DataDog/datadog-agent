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

func encodeEnvVar(e corev1.EnvVar) string {
	out, _ := json.Marshal(e)
	return string(out)
}

func findServiceNameEnvVarsInPod(pod *corev1.Pod) []corev1.EnvVar {
	// grouped by env var for DD_SERVICE,
	// all of the env var definitions and each container.
	//
	// we hold onto all of these because we should at least log
	// a warning for when they are incompatible.
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

	for _, c := range pod.Spec.Containers {
		iterContainer(&c)
	}

	return found
}

func findServiceNameInPod(pod *corev1.Pod) (corev1.EnvVar, bool) {
	found := findServiceNameEnvVarsInPod(pod)

	var ok bool
	var env corev1.EnvVar
	if len(found) > 0 {
		env = found[0]
		ok = true
		if len(found) > 1 {
			log.Debug("more than one unique definition of service name found, choosing the first one")
		}
	}

	if !ok {
		log.Debug("no service env vars found in pod, checking owner name")
		name, err := getServiceNameFromPodOwnerName(pod)
		if err != nil {
			log.Debugf("could not get service name from fallback: %v", err)
		} else if name == "" {
			log.Debugf("fallback provided no error but empty service name")
		} else {
			env.Name = kubernetes.ServiceTagEnvVar
			env.Value = name
			ok = true
		}
	}

	return env, ok
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
