// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
)

func planEnvOverrides(session *patch.PodSession, id patch.ContainerID, overrides ...corev1.EnvVar) (bool, error) {
	container, err := session.ContainerSnapshot(id)
	if err != nil {
		return false, err
	}
	mutated := false
	for _, env := range overrides {
		matches, err := session.FindEnv(id, env.Name)
		if err != nil {
			return false, err
		}
		if len(matches) > 0 {
			for _, existing := range container.Env {
				if existing.Name == env.Name {
					mutated = mutated || existing.Value != env.Value
					break
				}
			}
			if err := session.SetEnvOccurrence(matches[0], env); err != nil {
				return false, err
			}
		} else {
			if _, err := session.EnsureEnvs([]patch.EnvInjection{{Container: id, Env: env}}); err != nil {
				return false, err
			}
			mutated = true
		}
	}
	return mutated, nil
}
