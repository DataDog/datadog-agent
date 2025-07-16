// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// this file contains utilities to work with GPU assignment to containers

//go:build linux_bpf && nvml

package gpu

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

var errCannotMatchDevice = errors.New("cannot find matching device")
var numberedResourceRegex = regexp.MustCompile(`^nvidia([0-9]+)$`)

func matchContainerDevices(container *workloadmeta.Container, devices []ddnvml.Device) ([]ddnvml.Device, error) {
	var filteredDevices []ddnvml.Device

	var multiErr error
	for _, resource := range container.ResolvedAllocatedResources {
		// Only consider NVIDIA GPUs
		if !gpuutil.IsNvidiaKubernetesResource(resource.Name) {
			continue
		}

		matchingDevice, err := findDeviceForResourceName(devices, resource.ID)
		if err != nil {
			multiErr = errors.Join(multiErr, err)
			continue
		}

		filteredDevices = append(filteredDevices, matchingDevice)
	}

	return filteredDevices, multiErr
}

func findDeviceForResourceName(devices []ddnvml.Device, resourceID string) (ddnvml.Device, error) {
	// The NVIDIA device plugin resource ID is the GPU UUID, while GKE
	// device plugin sets the ID as nvidiaX, where X is the GPU index.
	match := numberedResourceRegex.FindStringSubmatch(resourceID)
	if len(match) == 0 {
		// No match -> NVIDIA device plugin
		return findDeviceByUUID(devices, resourceID)
	}

	// Check if any of the devices are MIG devices
	for _, device := range devices {
		physicalDevice, isPhysicalDevice := device.(*ddnvml.PhysicalDevice)
		_, isMigDevice := device.(*ddnvml.MIGDevice)
		if isMigDevice || (isPhysicalDevice && len(physicalDevice.MIGChildren) > 0) {
			return nil, fmt.Errorf("MIG devices are not supported for GKE device plugin")
		}
	}

	// Match -> GKE device plugin
	deviceIndex, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse device index from resource name %s: %w", resourceID, err)
	}
	return findDeviceByIndex(devices, deviceIndex)
}

func findDeviceByUUID(devices []ddnvml.Device, uuid string) (ddnvml.Device, error) {
	for _, device := range devices {
		if device.GetDeviceInfo().UUID == uuid {
			return device, nil
		}

		// If the device has MIG children, check if any of them matches the resource ID
		physicalDevice, ok := device.(*ddnvml.PhysicalDevice)
		if ok {
			for _, migChild := range physicalDevice.MIGChildren {
				if uuid == migChild.UUID {
					return migChild, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("%w with uuid %s", errCannotMatchDevice, uuid)
}

func findDeviceByIndex(devices []ddnvml.Device, index int) (ddnvml.Device, error) {
	for _, device := range devices {
		if device.GetDeviceInfo().Index == index {
			return device, nil
		}
	}

	return nil, fmt.Errorf("%w with index %d", errCannotMatchDevice, index)
}
