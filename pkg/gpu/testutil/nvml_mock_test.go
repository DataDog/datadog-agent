// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package testutil

import (
	"encoding/binary"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestMockNVMLCanonicalDeviceIdentity(t *testing.T) {
	mock := NewMockNVML()

	device, ret := mock.DeviceGetHandleByIndex(0)
	require.Equal(t, nvml.SUCCESS, ret)
	require.Same(t, mock.Device(0), device)

	device, ret = mock.DeviceGetHandleByIndex(-1)
	require.Equal(t, nvml.ERROR_INVALID_ARGUMENT, ret)
	require.Nil(t, device)

	require.Panics(t, func() { mock.Device(-1) })
	require.Panics(t, func() { mock.Device(len(GPUUUIDs)) })
}

func TestMockNVMLCanonicalMIGDeviceIdentity(t *testing.T) {
	mock := NewMockNVML(WithDeviceOptions(
		DefaultMIGParentDeviceIdx,
		WithMIGEnabled(),
		WithMIGChildUUIDs(MIGChildrenUUIDs[DefaultMIGParentDeviceIdx]),
	))
	parent := mock.Device(DefaultMIGParentDeviceIdx)
	require.NotNil(t, parent)

	child, ret := parent.GetMigDeviceHandleByIndex(0)
	require.Equal(t, nvml.SUCCESS, ret)
	require.Same(t, mock.MIGDevice(DefaultMIGParentDeviceIdx, 0), child)
	require.Panics(t, func() { mock.MIGDevice(-1, 0) })
	require.Panics(t, func() { mock.MIGDevice(DefaultMIGParentDeviceIdx, -1) })
	require.Panics(t, func() { mock.MIGDevice(0, 0) })
	require.Panics(t, func() { mock.MIGDevice(DefaultMIGParentDeviceIdx, 99) })
}

func TestMockNVMLDeviceOptionPrecedence(t *testing.T) {
	withName := func(name string) NvmlDeviceOption {
		return WithCustomHook(func(device *MockDevice) {
			device.GetNameFunc = func() (string, nvml.Return) {
				return name, nvml.SUCCESS
			}
		})
	}

	mock := NewMockNVML(
		WithDefaultMIGDevices(),
		withName("all-devices"),
		WithDeviceOptions(0, withName("device-zero")),
		WithDeviceOptions(DefaultMIGParentDeviceIdx, withName("mig-parent")),
	)

	name, ret := mock.Device(0).GetName()
	require.Equal(t, nvml.SUCCESS, ret)
	require.Equal(t, "device-zero", name)

	name, ret = mock.Device(1).GetName()
	require.Equal(t, nvml.SUCCESS, ret)
	require.Equal(t, "all-devices", name)

	name, ret = mock.MIGDevice(DefaultMIGParentDeviceIdx, 0).GetName()
	require.Equal(t, nvml.SUCCESS, ret)
	require.Equal(t, "mig-parent", name)
}

func TestMockNVMLFieldState(t *testing.T) {
	const fieldID = 123
	values := map[uint32]MockFieldValue{fieldID: NewFieldValue(10)}
	mock := NewMockNVML(WithDeviceOptions(0, WithFieldValuesFullOverride(values)))
	values[fieldID] = NewFieldValue(20)

	fields := []nvml.FieldValue{{FieldId: fieldID}}
	require.Equal(t, nvml.SUCCESS, mock.Device(0).GetFieldValues(fields))
	require.Equal(t, uint64(10), binary.LittleEndian.Uint64(fields[0].Value[:]))

	mock.Device(0).SetFieldValues(map[uint32]MockFieldValue{fieldID: NewFieldValue(30)})
	require.Equal(t, nvml.SUCCESS, mock.Device(0).GetFieldValues(fields))
	require.Equal(t, uint64(30), binary.LittleEndian.Uint64(fields[0].Value[:]))
}

func TestMockNVMLGpmMetricValues(t *testing.T) {
	values := map[nvml.GpmMetricId]MockGpmMetricValue{
		1: {Value: 42, Return: nvml.SUCCESS},
		2: {Return: nvml.ERROR_NOT_SUPPORTED},
	}
	mock := NewMockNVML(WithGpmMetricValues(values))
	values[1] = MockGpmMetricValue{Value: 99, Return: nvml.SUCCESS}

	metrics := nvml.GpmMetricsGetType{NumMetrics: 3}
	metrics.Metrics[0].MetricId = 1
	metrics.Metrics[1].MetricId = 2
	metrics.Metrics[2].MetricId = 3

	require.Equal(t, nvml.SUCCESS, mock.GpmMetricsGet(&metrics))
	require.Equal(t, 42.0, metrics.Metrics[0].Value)
	require.Equal(t, uint32(nvml.SUCCESS), metrics.Metrics[0].NvmlReturn)
	require.Equal(t, uint32(nvml.ERROR_NOT_SUPPORTED), metrics.Metrics[1].NvmlReturn)
	require.Equal(t, 0.0, metrics.Metrics[2].Value)
	require.Equal(t, uint32(nvml.SUCCESS), metrics.Metrics[2].NvmlReturn)
}
