// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package imageresolver provides configuration and utilities for resolving
// container image references from mutable tags to digests.
package imageresolver

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/opencontainers/go-digest"
)

func newDatadoghqRegistries(datadogRegistriesList []string) map[string]struct{} {
	datadoghqRegistries := make(map[string]struct{})
	for _, registry := range datadogRegistriesList {
		datadoghqRegistries[registry] = struct{}{}
	}
	return datadoghqRegistries
}

func isDatadoghqRegistry(registry string, datadoghqRegistries map[string]struct{}) bool {
	_, exists := datadoghqRegistries[registry]
	return exists
}

// isValidDigest validates that a digest string follows the OCI image specification format
// and is a sha256 digest.
func isValidDigest(digestStr string) bool {
	if _, err := digest.Parse(digestStr); err != nil {
		return false
	}
	return strings.HasPrefix(digestStr, "sha256:")
}

func parseAndValidateConfigs(configs map[string]state.RawConfig) (map[string]RepositoryConfig, map[string]error) {
	validConfigs := make(map[string]RepositoryConfig)
	errors := make(map[string]error)
	for configKey, rawConfig := range configs {
		var repo RepositoryConfig
		if err := json.Unmarshal(rawConfig.Config, &repo); err != nil {
			errors[configKey] = fmt.Errorf("failed to unmarshal: %w", err)
			continue
		}

		if repo.RepositoryName == "" {
			errors[configKey] = fmt.Errorf("missing repository_name in config %s", configKey)
			continue
		}
		validConfigs[configKey] = repo
	}
	return validConfigs, errors
}
