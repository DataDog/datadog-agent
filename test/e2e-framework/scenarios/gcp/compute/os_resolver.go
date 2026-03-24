// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
)

type imageResolveFunc func(e gcp.Environment, osInfo os.Descriptor) (imageInfo, error)

var imageResolvers = map[os.Flavor]imageResolveFunc{
	os.Ubuntu: resolveUbuntuImage,
	os.RedHat: resolveRhelImage,
}

func resolveUbuntuImage(_ gcp.Environment, osInfo os.Descriptor) (imageInfo, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.UbuntuDefault.Version
	}

	switch osInfo.Version {
	case os.Ubuntu2204.Version:
		return imageInfo{
			name:                      "ubuntu-2204-jammy-v20240904",
			disableUnattendedUpgrades: true,
		}, nil
	default:
		return imageInfo{}, nil
	}
}
func resolveRhelImage(_ gcp.Environment, osInfo os.Descriptor) (imageInfo, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.RedHatDefault.Version
	}
	switch osInfo.Version {
	case os.RedHat9.Version:
		return imageInfo{name: "rhel-9-v20250611"}, nil
	}

	return imageInfo{}, nil
}
