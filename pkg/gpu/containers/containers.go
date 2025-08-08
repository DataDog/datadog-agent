// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// Package containers has utilities to work with GPU assignment to containers
package containers

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ErrCannotMatchDevice is returned when a device cannot be matched to a container
var ErrCannotMatchDevice = errors.New("cannot find matching device")
var numberedResourceRegex = regexp.MustCompile(`^nvidia([0-9]+)$`)

const (
	nvidiaVisibleDevicesEnvVar = "NVIDIA_VISIBLE_DEVICES"
)

// HasGPUs returns true if the container has GPUs assigned to it.
func HasGPUs(container *workloadmeta.Container) bool {
	switch container.Runtime {
	case workloadmeta.ContainerRuntimeDocker:
		// If we have an error, we assume there are no GPUs for the container, so
		// ignore it.
		envVar, _ := getDockerVisibleDevicesEnv(container)
		return envVar != ""
	default:
		// We have no specific support for other runtimes, so fall back to the Kubernetes device
		// assignment if it's there
		for _, resource := range container.ResolvedAllocatedResources {
			if gpuutil.IsNvidiaKubernetesResource(resource.Name) {
				return true
			}
		}
		return false
	}
}

// MatchContainerDevices matches the devices assigned to a container to the list of available devices
// It returns a list of devices that are assigned to the container, and an error if any of the devices cannot be matched
func MatchContainerDevices(container *workloadmeta.Container, devices []ddnvml.Device) ([]ddnvml.Device, error) {
	switch container.Runtime {
	case workloadmeta.ContainerRuntimeDocker:
		return matchDockerDevices(container, devices)
	default:
		// We have no specific support for other runtimes, so fall back to the Kubernetes device
		// assignment if it's there
		return matchKubernetesDevices(container, devices)
	}
}

func getDockerVisibleDevicesEnv(container *workloadmeta.Container) (string, error) {
	// We can't use container.EnvVars as it doesn't contain the environment variables
	// added by the container runtime. We need to get them from the main PID environment.
	envVar, err := kernel.GetProcessEnvVariable(container.PID, kernel.ProcFSRoot(), nvidiaVisibleDevicesEnvVar)
	if err != nil {
		return "", fmt.Errorf("error getting %s for container %s: %w", nvidiaVisibleDevicesEnvVar, container.ID, err)
	}
	return strings.TrimSpace(envVar), nil
}

func matchDockerDevices(container *workloadmeta.Container, devices []ddnvml.Device) ([]ddnvml.Device, error) {
	var filteredDevices []ddnvml.Device
	var multiErr error

	visibleDevicesVar, err := getDockerVisibleDevicesEnv(container)
	if err != nil {
		return nil, err
	}

	if visibleDevicesVar == "" {
		return nil, fmt.Errorf("%s is not set, can't match devices", nvidiaVisibleDevicesEnvVar)
	}

	if visibleDevicesVar == "all" {
		return devices, nil
	}

	visibleDevices := strings.Split(visibleDevicesVar, ",")
	for _, device := range visibleDevices {
		matchingDevice, err := findDeviceByIndex(devices, device)
		if err != nil {
			multiErr = errors.Join(multiErr, err)
			continue
		}
		filteredDevices = append(filteredDevices, matchingDevice)
	}

	return filteredDevices, multiErr
}

func matchKubernetesDevices(container *workloadmeta.Container, devices []ddnvml.Device) ([]ddnvml.Device, error) {
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
	return findDeviceByIndex(devices, match[1])
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

	return nil, fmt.Errorf("%w with uuid %s", ErrCannotMatchDevice, uuid)
}

func findDeviceByIndex(devices []ddnvml.Device, index string) (ddnvml.Device, error) {
	indexInt, err := strconv.Atoi(index)
	if err != nil {
		return nil, fmt.Errorf("invalid device index %s: %w", index, err)
	}

	for _, device := range devices {
		if device.GetDeviceInfo().Index == indexInt {
			return device, nil
		}
	}

	return nil, fmt.Errorf("%w with index %s", ErrCannotMatchDevice, index)
}
