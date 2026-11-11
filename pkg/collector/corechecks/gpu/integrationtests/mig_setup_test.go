// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

const (
	// configureMIGEnv makes the MIG tests reconfigure the host: MIG mode is enabled on every
	// MIG-capable GPU and MIG devices are created, then the original configuration is restored when
	// the test ends. Without it the MIG tests only run against a host that is already in MIG mode.
	// Reconfiguring needs root and GPUs that are not in use.
	configureMIGEnv = "GPU_TEST_CONFIGURE_MIG"

	// migInstancesPerGPU is the number of MIG devices created on each GPU. More than one so that
	// parents with several children are covered.
	migInstancesPerGPU = 2

	// migDestroyRetries is how many times, one second apart, destroying a MIG instance or disabling MIG mode is retried while the GPU is in use.
	migDestroyRetries = 30
)

// configureMIGRequested parses configureMIGEnv as a boolean, so that "0" or "false" do not
// reconfigure the host. An unparsable value fails the test rather than silently picking a side.
func configureMIGRequested(t *testing.T) bool {
	t.Helper()
	value := os.Getenv(configureMIGEnv)
	if value == "" {
		return false
	}
	requested, err := strconv.ParseBool(value)
	require.NoError(t, err, "%s must be a boolean, got %q", configureMIGEnv, value)
	return requested
}

// migConfigurationState records what configureMIG changed on a GPU, so it can be undone.
type migConfigurationState struct {
	uuid string
	// enabledMIGMode is true if MIG mode was off and configureMIG turned it on.
	enabledMIGMode bool
	// createdMIGDevices is true if the GPU had no MIG devices and configureMIG created some, so the cleanup
	// destroys every MIG device of the GPU. It is set before creating them, so that a failure while creating
	// them is cleaned up too.
	createdMIGDevices bool
}

// configureMIG enables MIG mode on every MIG-capable GPU that does not have it and creates MIG devices
// on the GPUs that have none, using NVML directly. GPUs that already have MIG devices are left as they
// are. The changes are reverted when the test ends. It fails the test if no GPU could be configured.
func configureMIG(t *testing.T) {
	t.Helper()

	requireNVMLSuccess(t, nvml.Init(), "initialize NVML")
	defer nvml.Shutdown() //nolint:errcheck

	count, ret := nvml.DeviceGetCount()
	requireNVMLSuccess(t, ret, "get device count")

	var states []*migConfigurationState
	// Registered before the loop so that a failure halfway through still restores the GPUs
	// configured so far.
	t.Cleanup(func() { restoreMIGConfiguration(t, states) })

	for index := range count {
		device, ret := nvml.DeviceGetHandleByIndex(index)
		requireNVMLSuccess(t, ret, "get handle for GPU %d", index)
		uuid, ret := device.GetUUID()
		requireNVMLSuccess(t, ret, "get UUID for GPU %d", index)

		mode, _, ret := device.GetMigMode()
		if ret == nvml.ERROR_NOT_SUPPORTED {
			t.Logf("GPU %d (%s) does not support MIG, leaving it as is", index, uuid)
			continue
		}
		requireNVMLSuccess(t, ret, "get MIG mode of GPU %d", index)

		state := &migConfigurationState{uuid: uuid}
		states = append(states, state)

		if mode != nvml.DEVICE_MIG_ENABLE {
			t.Logf("enabling MIG mode on GPU %d (%s)", index, uuid)
			state.enabledMIGMode = true
			activation, ret := device.SetMigMode(nvml.DEVICE_MIG_ENABLE)
			requireNVMLSuccess(t, ret, "enable MIG mode on GPU %d", index)
			// The mode is applied immediately only if nothing is using the GPU; otherwise it stays
			// pending until a GPU reset or a reboot.
			requireNVMLSuccess(t, activation, "activate MIG mode on GPU %d (stop the processes using it and reset the GPU with `nvidia-smi --gpu-reset`; on a VM a reboot may be needed)", index)
		}

		existing, ret := device.GetMaxMigDeviceCount()
		requireNVMLSuccess(t, ret, "get MIG device count of GPU %d", index)
		hasMIGDevices := false
		for child := range existing {
			if _, ret := device.GetMigDeviceHandleByIndex(child); ret == nvml.SUCCESS {
				hasMIGDevices = true
				break
			}
		}
		if hasMIGDevices {
			t.Logf("GPU %d (%s) already has MIG devices, using them", index, uuid)
			continue
		}

		state.createdMIGDevices = true
		createMIGDevices(t, device, index)
	}
}

// createMIGDevices creates migInstancesPerGPU GPU instances with the smallest profile, each with a
// compute instance.
func createMIGDevices(t *testing.T, device nvml.Device, index int) {
	t.Helper()

	profile, ret := device.GetGpuInstanceProfileInfo(nvml.GPU_INSTANCE_PROFILE_1_SLICE)
	requireNVMLSuccess(t, ret, "get the 1-slice GPU instance profile of GPU %d", index)

	created := 0
	for range migInstancesPerGPU {
		if capacity, ret := device.GetGpuInstanceRemainingCapacity(&profile); ret != nvml.SUCCESS || capacity == 0 {
			break
		}
		instance, ret := device.CreateGpuInstance(&profile)
		requireNVMLSuccess(t, ret, "create a GPU instance on GPU %d", index)
		created++

		computeProfile, ret := instance.GetComputeInstanceProfileInfo(nvml.COMPUTE_INSTANCE_PROFILE_1_SLICE, nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_SHARED)
		requireNVMLSuccess(t, ret, "get the compute instance profile on GPU %d", index)
		_, ret = instance.CreateComputeInstance(&computeProfile)
		requireNVMLSuccess(t, ret, "create a compute instance on GPU %d", index)
	}
	require.NotZero(t, created, "could not create any MIG device on GPU %d", index)
}

// restoreMIGConfiguration destroys the MIG devices configureMIG created and turns MIG mode off on the
// GPUs where it turned it on. It runs in a cleanup, so failures are reported without stopping.
func restoreMIGConfiguration(t *testing.T, states []*migConfigurationState) {
	t.Helper()

	if len(states) == 0 {
		return
	}
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		t.Errorf("initialize NVML to restore the MIG configuration: %v", nvml.ErrorString(ret))
		return
	}
	defer nvml.Shutdown() //nolint:errcheck

	for _, state := range states {
		device, ret := nvml.DeviceGetHandleByUUID(state.uuid)
		if ret != nvml.SUCCESS {
			t.Errorf("get GPU %s to restore its MIG configuration: %v", state.uuid, nvml.ErrorString(ret))
			continue
		}

		if state.createdMIGDevices && !destroyMIGDevices(t, device, state.uuid) {
			// MIG mode cannot be disabled while MIG devices remain, so disabling it would only fail after
			// the retries and hide the error above.
			if state.enabledMIGMode {
				t.Errorf("not disabling MIG mode on GPU %s: its MIG devices could not be destroyed", state.uuid)
			}
			continue
		}
		if state.enabledMIGMode {
			activation, ret := device.SetMigMode(nvml.DEVICE_MIG_DISABLE)
			// The mode cannot change while a client, such as a process that was just killed, still holds the GPU.
			for range migDestroyRetries {
				if ret != nvml.SUCCESS || activation != nvml.ERROR_IN_USE {
					break
				}
				time.Sleep(time.Second)
				activation, ret = device.SetMigMode(nvml.DEVICE_MIG_DISABLE)
			}
			if ret != nvml.SUCCESS || activation != nvml.SUCCESS {
				t.Errorf("disable MIG mode on GPU %s (return %v, activation %v); it may need a GPU reset or a reboot",
					state.uuid, nvml.ErrorString(ret), nvml.ErrorString(activation))
			}
		}
	}
}

// destroyMIGDevices destroys every GPU instance of the smallest profile on the GPU, which are the ones
// createMIGDevices creates, along with their compute instances. It lists the instances instead of
// remembering the ones created, so it also removes those left by a partial creation. It returns whether
// every instance was destroyed.
func destroyMIGDevices(t *testing.T, device nvml.Device, uuid string) bool {
	t.Helper()

	profile, ret := device.GetGpuInstanceProfileInfo(nvml.GPU_INSTANCE_PROFILE_1_SLICE)
	if ret != nvml.SUCCESS {
		t.Errorf("get the 1-slice GPU instance profile of GPU %s: %v", uuid, nvml.ErrorString(ret))
		return false
	}
	instances, ret := device.GetGpuInstances(&profile)
	if ret != nvml.SUCCESS {
		t.Errorf("list the GPU instances of GPU %s: %v", uuid, nvml.ErrorString(ret))
		return false
	}
	ok := true
	for _, instance := range instances {
		ok = destroyGPUInstance(t, uuid, instance) && ok
	}
	return ok
}

// destroyGPUInstance destroys a GPU instance and its compute instances, and returns whether it succeeded.
func destroyGPUInstance(t *testing.T, uuid string, instance nvml.GpuInstance) bool {
	t.Helper()

	ok := true
	// A GPU instance can only be destroyed once its compute instances are gone.
	computeProfile, ret := instance.GetComputeInstanceProfileInfo(nvml.COMPUTE_INSTANCE_PROFILE_1_SLICE, nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_SHARED)
	if ret == nvml.SUCCESS {
		computeInstances, ret := instance.GetComputeInstances(&computeProfile)
		if ret != nvml.SUCCESS {
			t.Errorf("list the compute instances of a GPU instance on GPU %s: %v", uuid, nvml.ErrorString(ret))
			ok = false
		}
		for _, computeInstance := range computeInstances {
			if ret := retryWhileInUse(computeInstance.Destroy); ret != nvml.SUCCESS {
				t.Errorf("destroy a compute instance on GPU %s: %v", uuid, nvml.ErrorString(ret))
				ok = false
			}
		}
	}
	if ret := retryWhileInUse(instance.Destroy); ret != nvml.SUCCESS {
		t.Errorf("destroy a GPU instance on GPU %s: %v", uuid, nvml.ErrorString(ret))
		ok = false
	}
	return ok
}

// retryWhileInUse calls destroy until it stops reporting that a client is still using the instance.
// A process that was just killed takes a moment to release its GPU context.
func retryWhileInUse(destroy func() nvml.Return) nvml.Return {
	ret := destroy()
	for range migDestroyRetries {
		if ret != nvml.ERROR_IN_USE {
			break
		}
		time.Sleep(time.Second)
		ret = destroy()
	}
	return ret
}

func requireNVMLSuccess(t *testing.T, ret nvml.Return, format string, args ...any) {
	t.Helper()
	require.Equal(t, nvml.SUCCESS, ret, "%s: %s", fmt.Sprintf(format, args...), nvml.ErrorString(ret))
}
