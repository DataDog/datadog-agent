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
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ddInjectorDeviceName is the device name for the APM injector driver
	ddInjectorDeviceName = `\\.\DDInjector`
)

// maxKnownVersion is the highest counter contract version this agent knows how
// to decode. Bump this (and add the corresponding gauges) when teaching the
// agent about a new counter version.
const maxKnownVersion = CountersVersion2

// capabilitiesErrorLogged ensures the capabilities-query fallback is logged
// only once (older drivers without the capabilities IOCTL would otherwise spam
// the log on every collection).
var capabilitiesErrorLogged atomic.Bool

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

	// v2 fields (crash / boot-recovery telemetry). Populated only when the
	// driver negotiates counter version >= 2; otherwise they are reset to 0.
	CrashesDuringInjection          telemetry.SimpleGauge
	CrashesPostInjection            telemetry.SimpleGauge
	BootRecoveryCrashBootsDetected  telemetry.SimpleGauge
	BootRecoveryDriverSelfDisabled  telemetry.SimpleGauge
	BootRecoveryStabilityTimerFired telemetry.SimpleGauge
}

// Injector represents an opened instance to the ddinjector driver.
type Injector struct {
	handle windows.Handle
}

// function pointers to swap out driver-specific calls to facilitate unit testing
var doOpenDriverHandle = openDriverHandle
var doQueryDriverCapabilities = queryDriverCapabilities
var doQueryDriverCounters = queryDriverCounters

// NewInjector opens a handle to ddinjector to allow subsequent queries.
func NewInjector() (*Injector, error) {
	h, err := doOpenDriverHandle()
	if err != nil {
		return nil, fmt.Errorf("failed to open ddinjector: %w", err)
	}
	return &Injector{handle: h}, nil
}

// negotiateCounterVersion queries the driver capabilities and returns the
// highest counter version both the agent and the driver understand.
//
// Older drivers do not implement the capabilities IOCTL; in that case we fall
// back to V1 (the frozen baseline every driver supports) and log the first
// occurrence only.
func (inj *Injector) negotiateCounterVersion() uint32 {
	driverMax, err := doQueryDriverCapabilities(inj.handle)
	if err != nil {
		if capabilitiesErrorLogged.CompareAndSwap(false, true) {
			log.Warnf("ddinjector capabilities query failed, falling back to V1 counters (logging first error only): %v", err)
		}
		return CountersVersion1
	}

	return max(min(driverMax, maxKnownVersion), CountersVersion1)
}

// GetCounters queries the ddinjector current counters, negotiating the counter
// contract version with the driver and collecting whatever it supports.
func (inj *Injector) GetCounters(counters *InjectorCounters) error {
	negotiated := inj.negotiateCounterVersion()

	// A V2 buffer has the V1 counters at its head, so always read into the
	// widest known struct and tell the driver only how much we expect for the
	// negotiated version. This keeps a single read/validate path regardless of
	// version.
	raw := DDInjectorCountersV2{}
	expectedSize := uint32(DDInjectorCountersV1Size)
	if negotiated >= CountersVersion2 {
		expectedSize = uint32(DDInjectorCountersV2Size)
	}

	request := NewCounterRequest(negotiated)
	bytesReturned, err := doQueryDriverCounters(inj.handle, &request, unsafe.Pointer(&raw), expectedSize)
	if err != nil {
		return fmt.Errorf("ddinjector DeviceIoControl failed: %w", err)
	}
	if bytesReturned < expectedSize {
		return fmt.Errorf("ddinjector returned %d bytes, expected at least %d", bytesReturned, expectedSize)
	}

	// The V1 counters occupy the head of the buffer, so reinterpret it to
	// populate the V1 gauges without depending on cgo nested-field naming.
	populateV1Gauges(counters, (*DDInjectorCountersV1)(unsafe.Pointer(&raw)))
	if negotiated >= CountersVersion2 {
		populateV2Gauges(counters, &raw)
	} else {
		clearV2Gauges(counters)
	}
	return nil
}

// populateV1Gauges sets the V1 counter gauges from a raw V1 counter struct.
func populateV1Gauges(counters *InjectorCounters, raw *DDInjectorCountersV1) {
	counters.ProcessesAddedToInjectionTracker.Set(float64(raw.ProcessesAddedToInjectionTracker))
	counters.ProcessesRemovedFromInjectionTracker.Set(float64(raw.ProcessesRemovedFromInjectionTracker))
	counters.ProcessesSkippedSubsystem.Set(float64(raw.ProcessesSkippedSubsystem))
	counters.ProcessesSkippedContainer.Set(float64(raw.ProcessesSkippedContainer))
	counters.ProcessesSkippedProtected.Set(float64(raw.ProcessesSkippedProtected))
	counters.ProcessesSkippedSystem.Set(float64(raw.ProcessesSkippedSystem))
	counters.ProcessesSkippedExcluded.Set(float64(raw.ProcessesSkippedExcluded))
	counters.InjectionAttempts.Set(float64(raw.InjectionAttempts))
	counters.InjectionAttemptFailures.Set(float64(raw.InjectionAttemptFailures))
	counters.InjectionMaxTimeUs.Set(float64(raw.InjectionMaxTimeUs))
	counters.InjectionSuccesses.Set(float64(raw.InjectionSuccesses))
	counters.InjectionFailures.Set(float64(raw.InjectionFailures))
	counters.PeCachingFailures.Set(float64(raw.PeCachingFailures))
	counters.ImportDirectoryRestorationFailures.Set(float64(raw.ImportDirectoryRestorationFailures))
	counters.PeMemoryAllocationFailures.Set(float64(raw.PeMemoryAllocationFailures))
	counters.PeInjectionContextAllocated.Set(float64(raw.PeInjectionContextAllocated))
	counters.PeInjectionContextCleanedup.Set(float64(raw.PeInjectionContextCleanedup))
}

// populateV2Gauges sets the V2 counter gauges from a raw V2 counter struct.
func populateV2Gauges(counters *InjectorCounters, raw *DDInjectorCountersV2) {
	counters.CrashesDuringInjection.Set(float64(raw.CrashesDuringInjection))
	counters.CrashesPostInjection.Set(float64(raw.CrashesPostInjection))
	counters.BootRecoveryCrashBootsDetected.Set(float64(raw.BootRecoveryCrashBootsDetected))
	counters.BootRecoveryDriverSelfDisabled.Set(float64(raw.BootRecoveryDriverSelfDisabled))
	counters.BootRecoveryStabilityTimerFired.Set(float64(raw.BootRecoveryStabilityTimerFired))
}

// clearV2Gauges resets V2 gauges when the current driver contract is V1-only.
func clearV2Gauges(counters *InjectorCounters) {
	counters.CrashesDuringInjection.Set(0)
	counters.CrashesPostInjection.Set(0)
	counters.BootRecoveryCrashBootsDetected.Set(0)
	counters.BootRecoveryDriverSelfDisabled.Set(0)
	counters.BootRecoveryStabilityTimerFired.Set(0)
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

// queryDriverCapabilities calls DeviceIoControl on ddinjector to query the
// driver capabilities, returning the highest counter version the driver
// supports.
func queryDriverCapabilities(handle windows.Handle) (uint32, error) {
	var bytesReturned uint32

	if handle == windows.InvalidHandle || handle == 0 {
		return 0, errors.New("invalid handle to ddinjector, cannot query capabilities")
	}

	caps := DDInjectorCapabilities{}

	err := windows.DeviceIoControl(
		handle,
		GetCapabilitiesIOCTL,
		nil,                                // no input buffer
		0,                                  // no input
		(*byte)(unsafe.Pointer(&caps)),     // output buffer
		uint32(DDInjectorCapabilitiesSize), // output size
		&bytesReturned,
		nil, // not overlapped
	)
	if err != nil {
		return 0, err
	}
	if bytesReturned < uint32(DDInjectorCapabilitiesSize) {
		return 0, fmt.Errorf("ddinjector capabilities query returned %d bytes, expected at least %d", bytesReturned, DDInjectorCapabilitiesSize)
	}

	return uint32(caps.MaxSupportedCounterVersion), nil
}

// queryDriverCounters calls DeviceIoControl on ddinjector to query raw counters
// of the requested version into the supplied buffer. It returns the number of
// bytes the driver wrote so the caller can validate it against the expected
// struct size before trusting the buffer contents.
func queryDriverCounters(handle windows.Handle, request *DDInjectorCounterRequest, buffer unsafe.Pointer, bufferSize uint32) (uint32, error) {
	var bytesReturned uint32

	if handle == windows.InvalidHandle || handle == 0 {
		return 0, errors.New("invalid handle to ddinjector, cannot query")
	}

	err := windows.DeviceIoControl(
		handle,
		GetCountersIOCTL,
		(*byte)(unsafe.Pointer(request)), // input buffer
		uint32(unsafe.Sizeof(*request)),  // input size
		(*byte)(buffer),                  // output buffer
		bufferSize,                       // output size
		&bytesReturned,
		nil, // not overlapped
	)

	return bytesReturned, err
}
