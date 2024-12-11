// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// withEnvOverrides applies the extraEnv overrides to the container. Returns a
// boolean that indicates if the container was mutated
func withEnvOverrides(container *corev1.Container, extraEnv ...corev1.EnvVar) (bool, error) {
	if container == nil {
		return false, fmt.Errorf("can't apply environment overrides to nil container")
	}

	mutated := false

	for _, envVarOverride := range extraEnv {
		// Check if the environment variable already exists in the container
		var found bool
		for i, envVar := range container.Env {
			if envVar.Name == envVarOverride.Name {
				// Override the existing environment variable value
				container.Env[i] = envVarOverride
				found = true
				if envVar.Value != envVarOverride.Value {
					mutated = true
				}
				break
			}
		}
		// If the environment variable doesn't exist, add it to the container
		if !found {
			container.Env = append(container.Env, envVarOverride)
			mutated = true
		}
	}

	return mutated, nil
}

// withResourceLimits applies the resource limits overrides to the container
func withResourceLimits(container *corev1.Container, resourceLimits corev1.ResourceRequirements) error {
	if container == nil {
		return fmt.Errorf("can't apply resource requirements overrides to nil container")
	}
	container.Resources = resourceLimits
	return nil
}
