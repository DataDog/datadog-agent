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
	"time"
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

// capabilitiesErrorLogLimit throttles the capabilities-query fallback log so an
// older driver without the capabilities IOCTL does not spam the log on every
// collection, while still re-logging periodically so transient error streaks
// stay visible.
var capabilitiesErrorLogLimit = log.NewLogLimit(1, 10*time.Minute)

// unsupportedVersionLogLimit throttles the log emitted when the driver reports
// a counter version the agent cannot decode; the collection loop runs every few
// seconds, so an unthrottled log would spam.
var unsupportedVersionLogLimit = log.NewLogLimit(1, 10*time.Minute)

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
// highest counter version both the agent and the driver understand, or 0 when
// there is no common version and no counters should be collected.
//
// Older drivers do not implement the capabilities IOCTL; in that case we fall
// back to V1 (the frozen baseline every such driver supports) and log the
// failure at a throttled rate. A driver that answers the query but advertises a
// version below V1 is affirmatively reporting that it supports no counters we
// know, so we honor that and collect nothing rather than forcing a V1 read.
func (inj *Injector) negotiateCounterVersion() uint32 {
	driverMax, err := doQueryDriverCapabilities(inj.handle)
	if err != nil {
		if capabilitiesErrorLogLimit.ShouldLog() {
			log.Warnf("ddinjector capabilities query failed, falling back to V1 counters: %v", err)
		}
		return CountersVersion1
	}

	if driverMax < CountersVersion1 {
		if unsupportedVersionLogLimit.ShouldLog() {
			log.Warnf("ddinjector reported counter version %d below the V1 baseline, collecting no counters", driverMax)
		}
		return 0
	}
	return min(driverMax, maxKnownVersion)
}

// GetCounters queries the ddinjector current counters, negotiating the counter
// contract version with the driver and collecting whatever it supports.
func (inj *Injector) GetCounters(counters *InjectorCounters) error {
	negotiated := inj.negotiateCounterVersion()

	// The driver reports no counter version we understand; clear all gauges so
	// stale values are not reported and skip the counters query entirely.
	if negotiated == 0 {
		clearV1Gauges(counters)
		clearV2Gauges(counters)
		return nil
	}

	// A V2 buffer has the V1 counters at its head, so always read into the
	// widest known struct and tell the driver only how much we expect for the
	// negotiated version. This keeps a single read/validate path regardless of
	// version.
	raw := DDInjectorCountersV2{}
	expectedSize := uint32(DDInjectorCountersV1Size)
	if negotiated == CountersVersion2 {
		expectedSize = uint32(DDInjectorCountersV2Size)
	}

	request := NewCounterRequest(negotiated)
	bytesReturned, err := doQueryDriverCounters(inj.handle, &request, unsafe.Pointer(&raw), expectedSize)
	if err != nil {
		return fmt.Errorf("ddinjector DeviceIoControl failed: %w", err)
	}
	// Require at least the negotiated size rather than an exact match: a
	// forward-compatible driver may return a larger native struct, and we only
	// reinterpret the head we understand.
	if bytesReturned < expectedSize {
		return fmt.Errorf("ddinjector returned %d bytes, expected at least %d", bytesReturned, expectedSize)
	}

	// The V1 counters occupy the head of the buffer, so reinterpret it to
	// populate the V1 gauges without depending on cgo nested-field naming.
	populateV1Gauges(counters, (*DDInjectorCountersV1)(unsafe.Pointer(&raw)))
	if negotiated == CountersVersion2 {
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

// clearV1Gauges resets V1 gauges when the driver supports no counter version
// the agent can decode.
func clearV1Gauges(counters *InjectorCounters) {
	counters.ProcessesAddedToInjectionTracker.Set(0)
	counters.ProcessesRemovedFromInjectionTracker.Set(0)
	counters.ProcessesSkippedSubsystem.Set(0)
	counters.ProcessesSkippedContainer.Set(0)
	counters.ProcessesSkippedProtected.Set(0)
	counters.ProcessesSkippedSystem.Set(0)
	counters.ProcessesSkippedExcluded.Set(0)
	counters.InjectionAttempts.Set(0)
	counters.InjectionAttemptFailures.Set(0)
	counters.InjectionMaxTimeUs.Set(0)
	counters.InjectionSuccesses.Set(0)
	counters.InjectionFailures.Set(0)
	counters.PeCachingFailures.Set(0)
	counters.ImportDirectoryRestorationFailures.Set(0)
	counters.PeMemoryAllocationFailures.Set(0)
	counters.PeInjectionContextAllocated.Set(0)
	counters.PeInjectionContextCleanedup.Set(0)
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
