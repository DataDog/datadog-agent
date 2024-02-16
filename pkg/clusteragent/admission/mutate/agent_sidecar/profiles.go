// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	corev1 "k8s.io/api/core/v1"
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

// LoadSidecarProfiles returns the profile overrides provided by the user
// It returns an error in case of miss-configuration or in case more than
// one profile is configured
func LoadSidecarProfiles() ([]ProfileOverride, error) {
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

func applyProfileOverrides(container *corev1.Container) error {
	if container == nil {
		return fmt.Errorf("can't apply profile overrides to nil containers")
	}

	profiles, err := LoadSidecarProfiles()

	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return nil
	}

	overrides := profiles[0]

	// Apply environment variable overrides
	err = withEnvOverrides(container, overrides.EnvVars...)
	if err != nil {
		return err
	}

	// Apply resource requirement overrides
	if overrides.ResourceRequirements.Limits != nil {
		err = withResourceLimits(container, overrides.ResourceRequirements)
		if err != nil {
			return err
		}
	}

	return nil
}
