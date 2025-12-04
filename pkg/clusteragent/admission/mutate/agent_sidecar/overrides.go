// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

// withEnvOverrides applies the extraEnv overrides to the container. Returns a
// boolean that indicates if the container was mutated
func withEnvOverrides(container *corev1.Container, extraEnv ...corev1.EnvVar) (bool, error) {
	if container == nil {
		return false, errors.New("can't apply environment overrides to nil container")
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
			// Prepend rather than append the new variables so that they precede the previous ones in the final list,
			// allowing them to be referenced in other environment variables downstream.
			// (See: https://kubernetes.io/docs/tasks/inject-data-application/define-interdependent-environment-variables)
			container.Env = append([]corev1.EnvVar{envVarOverride}, container.Env...)
			mutated = true
		}
	}

	return mutated, nil
}

// withResourceLimits applies the resource limits overrides to the container
func withResourceLimits(container *corev1.Container, resourceLimits corev1.ResourceRequirements) error {
	if container == nil {
		return errors.New("can't apply resource requirements overrides to nil container")
	}
	container.Resources = resourceLimits
	return nil
}

// withSecurityContextOverrides applies the security context overrides to the container
func withSecurityContextOverrides(container *corev1.Container, securityContext *corev1.SecurityContext) (bool, error) {
	if container == nil {
		return false, errors.New("can't apply security context overrides to nil container")
	}

	mutated := false

	if securityContext != nil {
		container.SecurityContext = securityContext
		mutated = true
	}

	return mutated, nil
}
