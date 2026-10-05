// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
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
	SecurityContext      *corev1.SecurityContext     `json:"securityContext,omitempty"`
}

// loadSidecarProfiles returns the profile overrides provided by the user
// It returns an error in case of miss-configuration or in case more than
// one profile is configured
func loadSidecarProfiles(profilesJSON string) ([]ProfileOverride, error) {
	// Read and parse profiles
	var profiles []ProfileOverride

	err := json.Unmarshal([]byte(profilesJSON), &profiles)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profiles for admission controller agent sidecar injection: %s", err)
	}

	if len(profiles) > 1 {
		return nil, errors.New("only 1 profile is supported")
	}

	return profiles, nil
}

// applyProfileOverrides applies the profile overrides to the container. It
// returns a boolean that indicates if the container was mutated
func planProfileOverrides(session *patch.PodSession, id patch.ContainerID, profiles []ProfileOverride) (bool, error) {
	if profiles == nil {
		return false, errors.New("can't apply nil profiles")
	}
	if len(profiles) == 0 {
		return false, nil
	}
	overrides := profiles[0]
	mutated, err := planEnvOverrides(session, id, overrides.EnvVars...)
	if err != nil {
		return false, err
	}
	if overrides.ResourceRequirements.Limits != nil {
		if err := session.ConfigureResources(id, overrides.ResourceRequirements); err != nil {
			return false, err
		}
		mutated = true
	}
	if overrides.SecurityContext != nil {
		if err := session.ConfigureSecurityContext(id, overrides.SecurityContext); err != nil {
			return false, err
		}
		mutated = true
	}
	return mutated, nil
}
