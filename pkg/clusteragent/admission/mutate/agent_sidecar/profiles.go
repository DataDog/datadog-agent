// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
)

////////////////////////////////
//                            //
//     Profiles Overrides     //
//                            //
////////////////////////////////

// ProfileOverride represents environment variables and resource requirements overrides
// It returns an error in case it fails to apply the profile overrides
type ProfileOverride struct {
	EnvVars              []corev1.EnvVar             `json:"env,omitempty"`
	ResourceRequirements corev1.ResourceRequirements `json:"resources,omitempty"`
}

// loadSidecarProfiles returns the profile overrides provided by the user
// It returns an error in case of miss-configuration or in case more than
// one profile is configured
func loadSidecarProfiles() ([]ProfileOverride, error) {
	// Read and parse profiles
	profilesJSON := config.Datadog.GetString("admission_controller.agent_sidecar.profiles")

	var profiles []ProfileOverride

	err := json.Unmarshal([]byte(profilesJSON), &profiles)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profiles for admission controller agent sidecar injection: %s", err)
	}

	if len(profiles) > 1 {
		return nil, fmt.Errorf("only 1 profile is supported")
	}

	return profiles, nil
}

// applyProfileOverrides applies the profile overrides to the container. It
// returns a boolean that indicates if the container was mutated
func applyProfileOverrides(container *corev1.Container) (bool, error) {
	if container == nil {
		return false, fmt.Errorf("can't apply profile overrides to nil containers")
	}

	profiles, err := loadSidecarProfiles()

	if err != nil {
		return false, err
	}
	if len(profiles) == 0 {
		return false, nil
	}

	overrides := profiles[0]

	mutated := false

	// Apply environment variable overrides
	overridden, err := withEnvOverrides(container, overrides.EnvVars...)
	if err != nil {
		return false, err
	}
	mutated = mutated || overridden

	// Apply resource requirement overrides
	if overrides.ResourceRequirements.Limits != nil {
		err = withResourceLimits(container, overrides.ResourceRequirements)
		if err != nil {
			return mutated, err
		}
		mutated = true
	}

	return mutated, nil
}
