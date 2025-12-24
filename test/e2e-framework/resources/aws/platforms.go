// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// Handles AMIs for all OSes

// map[os][arch][version] = ami (e.g. map[ubuntu][x86_64][22.04] = "ami-01234567890123456")
type PlatformsAMIsType = map[string]string
type PlatformsArchsType = map[string]PlatformsAMIsType
type PlatformsType = map[string]PlatformsArchsType

//go:embed platforms.json
var platformsJSON []byte

var (
	platformsOnce sync.Once
	platforms     PlatformsType
	platformsErr  error
)

func getPlatforms() (PlatformsType, error) {
	platformsOnce.Do(func() {
		platforms = make(PlatformsType)
		platformsErr = json.Unmarshal(platformsJSON, &platforms)
	})
	return platforms, platformsErr
}

func GetAMI(descriptor *e2eos.Descriptor) (string, error) {
	platforms, err := getPlatforms()
	if err != nil {
		return "", fmt.Errorf("failed to load platforms.json: %w", err)
	}
	if _, ok := platforms[descriptor.Flavor.String()]; !ok {
		return "", fmt.Errorf("os '%s' not found in platforms.json", descriptor.Flavor.String())
	}
	if _, ok := platforms[descriptor.Flavor.String()][string(descriptor.Architecture)]; !ok {
		return "", fmt.Errorf("arch '%s' not found in platforms.json", descriptor.Architecture)
	}
	if _, ok := platforms[descriptor.Flavor.String()][string(descriptor.Architecture)][descriptor.Version]; !ok {
		return "", fmt.Errorf("version '%s' not found in platforms.json", descriptor.Version)
	}

	return platforms[descriptor.Flavor.String()][string(descriptor.Architecture)][descriptor.Version], nil
}
