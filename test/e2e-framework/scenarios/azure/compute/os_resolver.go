// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
)

type imageInfo struct {
	urn string
}

type imageResolveFunc func(azure.Environment, os.Descriptor) (string, error)

var imageResolvers = map[os.Flavor]imageResolveFunc{
	os.WindowsServer: resolveWindowsURN,
	os.WindowsClient: resolveWindowsClientURN,
	os.Ubuntu:        resolveUbuntuURN,
}

// resolveOS returns the image URN for the given OS.
func resolveOS(e azure.Environment, vmArgs vmArgs) (*imageInfo, error) {
	if vmArgs.imageURN == "" {
		if resolver, found := imageResolvers[vmArgs.osInfo.Flavor]; found {
			var err error
			vmArgs.imageURN, err = resolver(e, *vmArgs.osInfo)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unsupported OS flavor %v", vmArgs.osInfo.Flavor)
		}
	}

	imageInfo := &imageInfo{
		urn: vmArgs.imageURN,
	}

	return imageInfo, nil
}

func resolveWindowsURN(_ azure.Environment, osInfo os.Descriptor) (string, error) {
	if osInfo.Architecture == os.ARM64Arch {
		return "", errors.New("ARM64 is not supported for Windows")
	}
	if osInfo.Version == "" {
		osInfo.Version = os.WindowsServerDefault.Version
	}

	if osInfo.Version == "2016" || osInfo.Version == "2019" {
		// 2016 and 2019 uses a different URN format
		return fmt.Sprintf("MicrosoftWindowsServer:WindowsServer:%s-datacenter-gensecond:latest", osInfo.Version), nil
	}

	return fmt.Sprintf("MicrosoftWindowsServer:WindowsServer:%s-datacenter-azure-edition-core:latest", osInfo.Version), nil
}

func resolveWindowsClientURN(_ azure.Environment, osInfo os.Descriptor) (string, error) {
	if osInfo.Architecture == os.ARM64Arch {
		return "", errors.New("ARM64 is not supported for Windows")
	}
	if osInfo.Version == "" {
		osInfo.Version = os.WindowsServerDefault.Version
	}

	return fmt.Sprintf("MicrosoftWindowsDesktop:%s:latest", osInfo.Version), nil
}

func resolveUbuntuURN(_ azure.Environment, osInfo os.Descriptor) (string, error) {
	if osInfo.Version == "" {
		osInfo.Version = os.UbuntuDefault.Version
	}

	switch osInfo.Version {
	case os.Ubuntu2204.Version:
		return "canonical:0001-com-ubuntu-server-jammy:22_04-lts-gen2:latest", nil
	default:
		return "", fmt.Errorf("unsupported Ubuntu version %s", osInfo.Version)
	}
}
