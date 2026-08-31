// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// Package containers has utilities to work with GPU assignment to containers
package containers

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ErrCannotMatchDevice is returned when a device cannot be matched to a container
var ErrCannotMatchDevice = errors.New("cannot find matching device")
var numberedResourceRegex = regexp.MustCompile(`^nvidia([0-9]+)$`)
var draDeviceNameRegex = regexp.MustCompile(`^gpu-([0-9]+)$`)

const (
	nvidiaVisibleDevicesEnvVar = "NVIDIA_VISIBLE_DEVICES"
	dockerInspectTimeout       = 100 * time.Millisecond
)

// HasGPUs returns true if the container has GPUs assigned to it.
func HasGPUs(container *workloadmeta.Container) bool {
	// ECS: Check GPUDeviceIDs (populated by Docker collector for ECS only)
	if len(container.GPUDeviceIDs) > 0 {
		return true
	}

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
		// A Kubernetes node can run the Docker runtime (cri-dockerd), and there
		// GPUDeviceIDs means something different: the NVML collector fills it
		// for the container's DRA devices only. Treating that as the whole
		// allocation would drop any device-plugin GPU the same container holds,
		// so anything carrying Kubernetes allocated resources goes through the
		// union path regardless of runtime.
		if hasKubernetesGPUResources(container) {
			return matchKubernetesDevices(container, devices)
		}
		// ECS: GPUDeviceIDs (UUID format) is populated by the Docker collector
		// at discovery time. ECS uses the Docker runtime but needs UUID-based
		// matching, so it takes precedence over the env-var inspection.
		if len(container.GPUDeviceIDs) > 0 {
			return matchByGPUDeviceIDs(container.GPUDeviceIDs, devices)
		}
		return matchDockerDevices(container, devices)
	default:
		// We have no specific support for other runtimes, so fall back to the Kubernetes device
		// assignment if it's there
		return matchKubernetesDevices(container, devices)
	}
}

// hasKubernetesGPUResources reports whether the container carries NVIDIA GPUs
// in its Kubernetes allocated resources, which an ECS task never does.
func hasKubernetesGPUResources(container *workloadmeta.Container) bool {
	for _, resource := range container.ResolvedAllocatedResources {
		if gpuutil.IsNvidiaKubernetesResource(resource.Name) {
			return true
		}
	}
	return false
}

func getDockerVisibleDevicesEnv(container *workloadmeta.Container) (string, error) {
	// We can't use container.EnvVars as it doesn't contain the environment
	// variables added by the container runtime, we only get those defined by
	// the container image, and those can be overridden by the container
	// runtime. We need to get them from the main PID environment.
	envVar, err := kernel.GetProcessEnvVariable(container.PID, kernel.ProcFSRoot(), nvidiaVisibleDevicesEnvVar)
	if err == nil {
		return strings.TrimSpace(envVar), nil
	}

	// If we have an error (e.g, the agent does not have permissions to inspect
	// the process environment variables) fall back to the container runtime
	// data
	if container.Resources.GPURequest == nil {
		return "", nil // no GPUs requested, so no visible devices
	}

	if *container.Resources.GPURequest == workloadmeta.RequestAllGPUs {
		return "all", nil
	}

	// return 0,1,...numGpus-1 as the assumed visible devices variable,
	// that's how Docker assigns devices to containers, there's no exclusive
	// allocation.
	visibleDevices := make([]string, int(*container.Resources.GPURequest))
	for i := 0; i < int(*container.Resources.GPURequest); i++ {
		visibleDevices[i] = strconv.Itoa(i)
	}
	return strings.Join(visibleDevices, ","), nil
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

	visibleDevices := strings.SplitSeq(visibleDevicesVar, ",")
	for device := range visibleDevices {
		matchingDevice, err := findDeviceByIndex(devices, device)
		if err != nil {
			multiErr = errors.Join(multiErr, err)
			continue
		}
		filteredDevices = append(filteredDevices, matchingDevice)
	}

	return filteredDevices, multiErr
}

// matchByGPUDeviceIDs matches devices using GPUDeviceIDs from workloadmeta.
// This is used for ECS containers where GPU UUIDs are provided in the format
// "GPU-aec058b1-c18e-236e-c14d-49d2990fda0f".
// Special values:
//   - "all" returns all available devices (GPU sharing)
//   - "none", "void" returns empty slice (no GPU access)
//
// The order of devices is preserved from the input gpuDeviceIDs, as this matches
// the order CUDA will use when selecting devices.
func matchByGPUDeviceIDs(gpuDeviceIDs []string, devices []ddnvml.Device) ([]ddnvml.Device, error) {
	if len(gpuDeviceIDs) == 1 {
		switch gpuDeviceIDs[0] {
		case "all":
			return devices, nil
		case "none", "void":
			return nil, nil
		}
	}

	var filteredDevices []ddnvml.Device
	var multiErr error

	for _, id := range gpuDeviceIDs {
		// ECS provides GPU UUIDs in format "GPU-xxxx-xxxx-xxxx-xxxx"
		matchingDevice, err := findDeviceByUUID(devices, id)
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

	// The NVML collector publishes a Container<->GPU mapping for DRA devices
	// (RFC §4.4a), resolving them -- MIG instances included -- to their real
	// NVML UUID via the node-local CDI/mig-minors chain. The regex guess below
	// cannot do that on a MIG-partitioned node, so take the mapping first.
	// It is a union rather than a short-circuit: a container can hold both DRA
	// and device-plugin resources, and only the DRA ones appear in the mapping.
	mapped := len(container.GPUDeviceIDs)
	if mapped > 0 {
		matched, err := matchByGPUDeviceIDs(container.GPUDeviceIDs, devices)
		multiErr = errors.Join(multiErr, err)
		filteredDevices = append(filteredDevices, matched...)
	}

	// The mapping resolves one UUID per DRA device, so it accounts for every
	// DRA resource only when it has at least as many entries as there are DRA
	// resources. Suppressing on "the mapping is non-empty" instead would hide
	// the error for a pod whose second claim failed to resolve, leaving it
	// attributed to half its GPUs with no telemetry saying so.
	draResources := 0
	for _, resource := range container.ResolvedAllocatedResources {
		if resource.Name == string(gpuutil.GpuNvidiaDRA) {
			draResources++
		}
	}
	draFullyMapped := draResources > 0 && mapped >= draResources

	for _, resource := range container.ResolvedAllocatedResources {
		// Only consider NVIDIA GPUs
		if !gpuutil.IsNvidiaKubernetesResource(resource.Name) {
			continue
		}

		var matchingDevice ddnvml.Device
		var err error
		if resource.Name == string(gpuutil.GpuNvidiaDRA) {
			matchingDevice, err = findDeviceForDRAResourceName(devices, resource.ID)
		} else {
			matchingDevice, err = findDeviceForResourceName(devices, resource.ID)
		}
		if err != nil {
			// A DRA device the mapping already resolved is expected to fail
			// here -- the pool-scoped name ("gpu-0-mig-1g18gb-19-0") is not an
			// NVML index. Only report it when the mapping does not cover it.
			if !draFullyMapped || resource.Name != string(gpuutil.GpuNvidiaDRA) {
				multiErr = errors.Join(multiErr, err)
			}
			continue
		}

		if !slices.Contains(filteredDevices, matchingDevice) {
			filteredDevices = append(filteredDevices, matchingDevice)
		}
	}

	// K8s can return the devices in a random order. However, NVIDIA will see them exposed
	// based on their actual device index in the system. Ensure that order is respected.
	//
	// Stable, because Index is not a total order across device kinds: for a MIG
	// child it is the index within its parent, so two children of different
	// parents can tie and an unstable sort would order them arbitrarily between
	// runs.
	slices.SortStableFunc(filteredDevices, func(a, b ddnvml.Device) int {
		aInfo := a.GetDeviceInfo()
		bInfo := b.GetDeviceInfo()

		return cmp.Compare(aInfo.Index, bInfo.Index)
	})

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
			return nil, errors.New("MIG devices are not supported for GKE device plugin")
		}
	}

	// Match -> GKE device plugin
	return findDeviceByIndex(devices, match[1])
}

func findDeviceForDRAResourceName(devices []ddnvml.Device, deviceName string) (ddnvml.Device, error) {
	match := draDeviceNameRegex.FindStringSubmatch(deviceName)
	if len(match) == 0 {
		return nil, fmt.Errorf("%w with DRA device name %s", ErrCannotMatchDevice, deviceName)
	}

	// DRA device names are mapped to physical NVML indexes. This mapping does
	// not identify individual MIG devices.
	for _, device := range devices {
		physicalDevice, isPhysicalDevice := device.(*ddnvml.PhysicalDevice)
		_, isMigDevice := device.(*ddnvml.MIGDevice)
		if isMigDevice || (isPhysicalDevice && len(physicalDevice.MIGChildren) > 0) {
			return nil, errors.New("MIG devices are not supported for DRA index matching")
		}
	}

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

// IsDatadogAgentContainer checks if a container belong to the Datadog Agent (might be the agent, but also system-probe or other components)
func IsDatadogAgentContainer(wmeta workloadmeta.Component, container *workloadmeta.Container) bool {
	currentPID := os.Getpid()
	runningContainer, err := wmeta.GetContainerForProcess(strconv.Itoa(currentPID))
	if err != nil {
		return false // errors might happen if the process is not running in a container, or if the process is not accounted for by workloadmeta
	}

	// If the container is the same as the running container, it is a Datadog container
	if runningContainer.EntityID == container.EntityID {
		return true
	}

	// However, we might have multiple containers as part of the same pod (e.g., agent and system-probe)
	// In this case, we need to check if the container is part of the same pod as the running container
	// Compare the struct as a value, not as a pointer, because the pointers might be different if they're created
	// in different places
	return runningContainer.Owner != nil && container.Owner != nil && *runningContainer.Owner == *container.Owner
}
