// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package ddinjector provides an interface to the Windows ddinjector driver.
package ddinjector

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ddInjectorDeviceName is the device name for the APM injector driver
	ddInjectorDeviceName = `\\.\DDInjector`

	countersVersion = 1
)

// InjectorCounters encapsulates ddinjector counters to be reported upstream.
type InjectorCounters struct {
	// v1 fields
	ProcessesAddedToInjectionTracker     telemetry.SimpleGauge
	ProcessesRemovedFromInjectionTracker telemetry.SimpleGauge
	ProcessesSkippedSubsystem            telemetry.SimpleGauge
	ProcessesSkippedContainer            telemetry.SimpleGauge
	ProcessesSkippedProtected            telemetry.SimpleGauge
	ProcessesSkippedSystem               telemetry.SimpleGauge
	ProcessesSkippedExcluded             telemetry.SimpleGauge
	InjectionAttempts                    telemetry.SimpleGauge
	InjectionAttemptFailures             telemetry.SimpleGauge
	InjectionMaxTimeUs                   telemetry.SimpleGauge
	InjectionSuccesses                   telemetry.SimpleGauge
	InjectionFailures                    telemetry.SimpleGauge
	PeCachingFailures                    telemetry.SimpleGauge
	ImportDirectoryRestorationFailures   telemetry.SimpleGauge
	PeMemoryAllocationFailures           telemetry.SimpleGauge
	PeInjectionContextAllocated          telemetry.SimpleGauge
	PeInjectionContextCleanedup          telemetry.SimpleGauge
}

// Injector represents an opened instance to the ddinjector driver.
type Injector struct {
	handle windows.Handle
}

// function pointers to swap out driver-specific calls to facilitate unit testing
var doOpenDriverHandle = openDriverHandle
var doQueryDriverCounters = queryDriverCounters

// NewInjector opens a handle to ddinjector to allow subsequent queries.
func NewInjector() (*Injector, error) {
	h, err := doOpenDriverHandle()
	if err != nil {
		return nil, fmt.Errorf("failed to open ddinjector: %w", err)
	}
	return &Injector{handle: h}, nil
}

// GetCounters queries the ddinjector current counters.
func (inj *Injector) GetCounters(counters *InjectorCounters) error {
	request := DDInjectorCounterRequest{}
	request.RequestedVersion = countersVersion

	rawCounters := DDInjectorCountersV1{}

	err := doQueryDriverCounters(inj.handle, &request, &rawCounters)
	if err != nil {
		return fmt.Errorf("ddinjector DeviceIoControl failed: %w", err)
	}

	counters.ProcessesAddedToInjectionTracker.Set(float64(rawCounters.ProcessesAddedToInjectionTracker))
	counters.ProcessesRemovedFromInjectionTracker.Set(float64(rawCounters.ProcessesRemovedFromInjectionTracker))
	counters.ProcessesSkippedSubsystem.Set(float64(rawCounters.ProcessesSkippedSubsystem))
	counters.ProcessesSkippedContainer.Set(float64(rawCounters.ProcessesSkippedContainer))
	counters.ProcessesSkippedProtected.Set(float64(rawCounters.ProcessesSkippedProtected))
	counters.ProcessesSkippedSystem.Set(float64(rawCounters.ProcessesSkippedSystem))
	counters.ProcessesSkippedExcluded.Set(float64(rawCounters.ProcessesSkippedExcluded))
	counters.InjectionAttempts.Set(float64(rawCounters.InjectionAttempts))
	counters.InjectionAttemptFailures.Set(float64(rawCounters.InjectionAttemptFailures))
	counters.InjectionMaxTimeUs.Set(float64(rawCounters.InjectionMaxTimeUs))
	counters.InjectionSuccesses.Set(float64(rawCounters.InjectionSuccesses))
	counters.InjectionFailures.Set(float64(rawCounters.InjectionFailures))
	counters.PeCachingFailures.Set(float64(rawCounters.PeCachingFailures))
	counters.ImportDirectoryRestorationFailures.Set(float64(rawCounters.ImportDirectoryRestorationFailures))
	counters.PeMemoryAllocationFailures.Set(float64(rawCounters.PeMemoryAllocationFailures))
	counters.PeInjectionContextAllocated.Set(float64(rawCounters.PeInjectionContextAllocated))
	counters.PeInjectionContextCleanedup.Set(float64(rawCounters.PeInjectionContextCleanedup))

	return nil
}

// Close closes the handle to ddinjector.
func (inj *Injector) Close() error {
	if inj.handle != windows.InvalidHandle && inj.handle != windows.Handle(0) {
		err := windows.CloseHandle(inj.handle)
		if err != nil {
			return fmt.Errorf("failed to close driver handle: %w", err)
		}

		log.Debugf("ddinjector closed")
		inj.handle = windows.Handle(0)
	}
	return nil
}

// openInjectorDriverHandle opens a handle to the APM injector driver
func openDriverHandle() (windows.Handle, error) {
	// Use statshandle path similar to ddnpm driver pattern
	fullpath := ddInjectorDeviceName
	pFullPath, err := windows.UTF16PtrFromString(fullpath)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("failed to convert path to UTF16: %w", err)
	}

	log.Debugf("Opening ddinjector: %s", fullpath)

	handle, err := windows.CreateFile(
		pFullPath,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0, // no overlapped I/O for stats queries
		windows.Handle(0),
	)

	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("failed to open ddinjector (%s): %w", fullpath, err)
	}

	log.Debugf("ddinjector opened")
	return handle, nil
}

// queryDriverCounters calls DeviceIoControl on ddinjector to query raw counters.
func queryDriverCounters(handle windows.Handle, request *DDInjectorCounterRequest, counters *DDInjectorCountersV1) error {
	var bytesReturned uint32

	if handle == windows.InvalidHandle || handle == 0 {
		return errors.New("invalid handle to ddinjector, cannot query")
	}

	err := windows.DeviceIoControl(
		handle,
		GetCountersIOCTL,
		(*byte)(unsafe.Pointer(request)),  // input buffer
		uint32(unsafe.Sizeof(*request)),   // input size
		(*byte)(unsafe.Pointer(counters)), // output buffer
		uint32(unsafe.Sizeof(*counters)),  // output size
		&bytesReturned,
		nil, // not overlapped
	)

	return err
}
