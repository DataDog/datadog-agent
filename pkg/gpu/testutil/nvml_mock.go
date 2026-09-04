// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package testutil

import (
	"maps"
	"slices"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

// MockNVML is a fully configured NVML interface and its canonical device graph.
type MockNVML struct {
	nvmlmock.Interface

	devices    []*MockDevice
	migDevices map[int]map[int]*MockDevice
	gpmState   *mockGpmState
}

// MockDevice is a configured NVML device with mutable test state.
type MockDevice struct {
	nvmlmock.Device

	fieldValuesMu     sync.RWMutex
	fieldValues       map[uint32]MockFieldValue
	scopedFieldValues map[uint32]map[uint32]MockFieldValue
}

type mockGpmState struct {
	mu         sync.Mutex
	allocCalls int
	freeCalls  int
	samples    []*MockGpmSample
}

type MockGpmSample struct {
	nvml.GpmSample
	ID       int
	GetIndex int
}

// NewMockNVML constructs the library, physical devices, and MIG devices.
// Device options are applied in this order: top-level defaults, then
// index-specific options.
func NewMockNVML(options ...NvmlMockOption) *MockNVML {
	opts := newNvmlMockOptions(options...)
	deviceCount := len(opts.physicalDeviceUUIDs)
	if deviceCount > len(GPUCores) {
		panic("NVML mock has more device UUIDs than GPU core counts")
	}

	for deviceIdx := range opts.deviceOptionsByIndex {
		if deviceIdx < 0 || deviceIdx >= deviceCount {
			panic("WithDeviceOptions device index is out of range")
		}
	}

	mock := &MockNVML{
		devices:    make([]*MockDevice, deviceCount),
		migDevices: make(map[int]map[int]*MockDevice),
		gpmState:   opts.gpmState,
	}

	for deviceIdx := 0; deviceIdx < deviceCount; deviceIdx++ {
		deviceOpts := newDeviceOptions(
			opts.defaultDeviceOptions,
			opts.physicalDeviceUUIDs[deviceIdx],
			opts.deviceOptionsByIndex[deviceIdx],
		)
		children := make(map[int]*MockDevice)
		childDevices := make(map[int]nvml.Device)
		for childIdx := range deviceOpts.migChildUUIDs {
			childOpts := withMIGChild(childIdx, deviceOpts.clone())
			child := newMockDevice(deviceIdx, childOpts, nil)
			children[childIdx] = child
			childDevices[childIdx] = child
		}
		if len(children) > 0 {
			mock.migDevices[deviceIdx] = children
		}
		mock.devices[deviceIdx] = newMockDevice(deviceIdx, deviceOpts, childDevices)
	}

	configureNVMLInterface(&mock.Interface, opts, mock.devices)
	return mock
}

// Device returns the canonical physical device for index and panics for an
// invalid index.
func (m *MockNVML) Device(index int) *MockDevice {
	return m.devices[index]
}

// MIGDevice returns the canonical MIG child and panics for an invalid parent or
// child index.
func (m *MockNVML) MIGDevice(parent, child int) *MockDevice {
	if m == nil {
		panic("MIGDevice called on nil MockNVML")
	}
	children, ok := m.migDevices[parent]
	if !ok {
		panic("invalid MIG parent index")
	}
	device, ok := children[child]
	if !ok {
		panic("invalid MIG child index")
	}
	return device
}

// SetFieldValues replaces the unscoped field values returned by the device.
func (m *MockDevice) SetFieldValues(values map[uint32]MockFieldValue) {
	m.fieldValuesMu.Lock()
	defer m.fieldValuesMu.Unlock()
	m.fieldValues = maps.Clone(values)
}

// GpmSampleAllocCount returns the number of GPM allocation calls, including
// a scripted failing call.
func (m *MockNVML) GpmSampleAllocCount() int {
	m.gpmState.mu.Lock()
	defer m.gpmState.mu.Unlock()
	return m.gpmState.allocCalls
}

// GpmSampleFreeCount returns the number of GPM free calls.
func (m *MockNVML) GpmSampleFreeCount() int {
	m.gpmState.mu.Lock()
	defer m.gpmState.mu.Unlock()
	return m.gpmState.freeCalls
}

// GpmSamples returns the successfully allocated identifiable samples.
func (m *MockNVML) GpmSamples() []*MockGpmSample {
	m.gpmState.mu.Lock()
	defer m.gpmState.mu.Unlock()
	return slices.Clone(m.gpmState.samples)
}

func newDeviceOptions(defaults deviceOptions, uuid string, overrides []NvmlDeviceOption) deviceOptions {
	options := defaults.clone()
	if options.uuid == nil {
		options.uuid = &uuid
	}
	options.apply(overrides...)
	return options.clone()
}

func newMockDevice(deviceIdx int, options deviceOptions, migDevices map[int]nvml.Device) *MockDevice {
	device := &MockDevice{
		fieldValues:       maps.Clone(options.fieldValues),
		scopedFieldValues: make(map[uint32]map[uint32]MockFieldValue, len(options.scopedFieldValues)),
	}
	for fieldID, values := range options.scopedFieldValues {
		device.scopedFieldValues[fieldID] = maps.Clone(values)
	}
	configureDeviceMock(device, deviceIdx, options, migDevices)
	return device
}

func (o *deviceOptions) apply(options ...NvmlDeviceOption) {
	for _, option := range options {
		option.applyDevice(o)
	}
}

func (o deviceOptions) clone() deviceOptions {
	cloned := o
	cloned.compatibilityHooks = slices.Clone(o.compatibilityHooks)
	cloned.fieldValues = maps.Clone(o.fieldValues)
	cloned.nvlinkStates = slices.Clone(o.nvlinkStates)
	cloned.nvlinkStateErrors = maps.Clone(o.nvlinkStateErrors)
	if o.scopedFieldValues != nil {
		cloned.scopedFieldValues = make(map[uint32]map[uint32]MockFieldValue, len(o.scopedFieldValues))
		for fieldID, values := range o.scopedFieldValues {
			cloned.scopedFieldValues[fieldID] = maps.Clone(values)
		}
	}
	if o.migChildUUIDs != nil {
		cloned.migChildUUIDs = maps.Clone(o.migChildUUIDs)
	}
	if o.processDetailList != nil {
		response := *o.processDetailList
		response.processes = slices.Clone(o.processDetailList.processes)
		cloned.processDetailList = &response
	}

	return cloned
}

func (m *MockDevice) getFieldValue(fieldID, scopeID uint32) *MockFieldValue {
	m.fieldValuesMu.RLock()
	defer m.fieldValuesMu.RUnlock()
	if scopedValues, ok := m.scopedFieldValues[fieldID]; ok {
		if value, ok := scopedValues[scopeID]; ok {
			return &value
		}
	}
	if value, ok := m.fieldValues[fieldID]; ok {
		return &value
	}
	return nil
}
