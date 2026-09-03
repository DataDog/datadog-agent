// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package agentcrashdetectimpl

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/time/rate"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	etw "github.com/DataDog/datadog-agent/comp/etw/def"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ddInjectorCrashEventType     = "ddinjector-crash"
	ddInjectorProviderGUID       = "{9933a039-281b-4342-a4e0-7109c8d3f22c}"
	ddInjectorETWSessionName     = "Datadog DDInjector crash telemetry"
	ddInjectorETWCrashEventName  = "CrashAttribution_Event"
	ddInjectorETWCrashKeyword    = uint64(0x40)
	ddInjectorCrashQueueSize     = 64
	ddInjectorCrashEventsPerMin  = 10
	traceEventInfoEventNameIndex = 92 // offsetof(TRACE_EVENT_INFO, EventNameOffset)
)

type ddInjectorCrashEvent struct {
	ProcessName      string `json:"process_name,omitempty"`
	ProcessID        uint32 `json:"process_id"`
	ExitStatus       string `json:"exit_status"`
	ElapsedMs        int64  `json:"elapsed_ms"`
	Phase            string `json:"phase"`
	EventsSuppressed uint64 `json:"events_suppressed,omitempty"`
}

var (
	tdhDLL                     = windows.NewLazySystemDLL("tdh.dll")
	tdhGetEventInformationProc = tdhDLL.NewProc("TdhGetEventInformation")
	tdhGetPropertySizeProc     = tdhDLL.NewProc("TdhGetPropertySize")
	tdhGetPropertyProc         = tdhDLL.NewProc("TdhGetProperty")
)

type tdhPropertyDataDescriptor struct {
	propertyName uint64
	arrayIndex   uint32
	reserved     uint32
}

type ddInjectorCrashListener struct {
	config compsysconfig.Component
	atel   agenttelemetry.Component
	etw    etw.Component
	log    corelog.Component

	providerGUID windows.GUID
	limiter      *rate.Limiter
	decodeErrors *utillog.Limit
	suppressed   atomic.Uint64

	session    etw.Session
	events     chan ddInjectorCrashEvent
	traceDone  chan struct{}
	workerDone chan struct{}
}

func newDDInjectorCrashListener(
	config compsysconfig.Component,
	atel agenttelemetry.Component,
	etwComponent etw.Component,
	log corelog.Component,
) *ddInjectorCrashListener {
	providerGUID, _ := windows.GUIDFromString(ddInjectorProviderGUID)
	return &ddInjectorCrashListener{
		config:       config,
		atel:         atel,
		etw:          etwComponent,
		log:          log,
		providerGUID: providerGUID,
		limiter: rate.NewLimiter(
			rate.Every(time.Minute/ddInjectorCrashEventsPerMin),
			ddInjectorCrashEventsPerMin,
		),
		decodeErrors: utillog.NewLogLimit(1, 10*time.Minute),
	}
}

func (l *ddInjectorCrashListener) start() error {
	if !l.config.GetBool("injector.enable_telemetry") {
		return nil
	}

	session, err := l.etw.NewSession(ddInjectorETWSessionName, func(_ *etw.SessionConfiguration) {})
	if err != nil {
		l.log.Warnf("Could not create the DDInjector crash telemetry ETW session: %v", err)
		return nil
	}

	session.ConfigureProvider(l.providerGUID, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_WARNING
		cfg.MatchAnyKeyword = ddInjectorETWCrashKeyword
	})
	if err = session.EnableProvider(l.providerGUID); err != nil {
		l.log.Warnf("Could not enable the DDInjector crash telemetry ETW provider: %v", err)
		_ = session.StopTracing()
		return nil
	}

	l.session = session
	l.events = make(chan ddInjectorCrashEvent, ddInjectorCrashQueueSize)
	l.traceDone = make(chan struct{})
	l.workerDone = make(chan struct{})

	go l.runWorker()
	go l.runTrace()
	l.log.Info("DDInjector per-crash telemetry is enabled")
	return nil
}

func (l *ddInjectorCrashListener) runTrace() {
	defer close(l.traceDone)
	if err := l.session.StartTracing(l.handleEvent); err != nil {
		l.log.Warnf("DDInjector crash telemetry ETW tracing stopped unexpectedly: %v", err)
	}
}

func (l *ddInjectorCrashListener) handleEvent(record *etw.DDEventRecord) {
	eventName, err := tdhEventName(record)
	if err != nil {
		l.logDecodeError("event name", err)
		return
	}
	if eventName != ddInjectorETWCrashEventName {
		return
	}

	event, err := decodeDDInjectorCrashEvent(record)
	if err != nil {
		l.logDecodeError("properties", err)
		return
	}
	if !l.limiter.Allow() {
		l.suppressed.Add(1)
		return
	}

	event.EventsSuppressed = l.suppressed.Swap(0)
	select {
	case l.events <- event:
	default:
		// Restore the prior suppressed count and include the event that could not be queued.
		l.suppressed.Add(event.EventsSuppressed + 1)
	}
}

func (l *ddInjectorCrashListener) logDecodeError(part string, err error) {
	if l.decodeErrors.ShouldLog() {
		l.log.Warnf("Could not decode DDInjector crash ETW event %s: %v", part, err)
	}
}

func (l *ddInjectorCrashListener) runWorker() {
	defer close(l.workerDone)
	for event := range l.events {
		payload, err := json.Marshal(event)
		if err != nil {
			l.log.Debugf("Could not marshal DDInjector crash telemetry: %v", err)
			continue
		}
		if err = l.atel.SendEvent(ddInjectorCrashEventType, payload); err != nil {
			l.log.Debugf("Could not send DDInjector crash telemetry: %v", err)
		}
	}
}

func (l *ddInjectorCrashListener) stop(ctx context.Context) error {
	if l.session == nil {
		return nil
	}

	if err := l.session.StopTracing(); err != nil {
		l.log.Debugf("Could not stop the DDInjector crash telemetry ETW session: %v", err)
	}

	select {
	case <-l.traceDone:
	case <-ctx.Done():
		return nil
	}
	close(l.events)
	select {
	case <-l.workerDone:
	case <-ctx.Done():
	}
	return nil
}

func decodeDDInjectorCrashEvent(record *etw.DDEventRecord) (ddInjectorCrashEvent, error) {
	messageBytes, err := tdhEventProperty(record, "Message")
	if err != nil {
		return ddInjectorCrashEvent{}, fmt.Errorf("Message: %w", err)
	}
	message := string(messageBytes)
	if end := strings.IndexByte(message, 0); end >= 0 {
		message = message[:end]
	}
	phase, ok := ddInjectorCrashPhase(message)
	if !ok {
		return ddInjectorCrashEvent{}, fmt.Errorf("unexpected Message value %q", message)
	}

	processID, err := tdhUint32Property(record, "ProcessId")
	if err != nil {
		return ddInjectorCrashEvent{}, err
	}
	exitStatus, err := tdhUint32Property(record, "ExitStatus")
	if err != nil {
		return ddInjectorCrashEvent{}, err
	}
	elapsedMs, err := tdhInt64Property(record, "ElapsedMs")
	if err != nil {
		return ddInjectorCrashEvent{}, err
	}

	processName := ""
	processNameBytes, err := tdhEventProperty(record, "ProcessName")
	if err == nil {
		processNameBytes, err = countedWideStringContents(processNameBytes)
		if err != nil {
			return ddInjectorCrashEvent{}, fmt.Errorf("ProcessName: %w", err)
		}
		if len(processNameBytes) > 0 {
			utf16Name := unsafe.Slice((*uint16)(unsafe.Pointer(&processNameBytes[0])), len(processNameBytes)/2)
			processName = processBaseName(windows.UTF16ToString(utf16Name))
		}
	} else if !errors.Is(err, windows.ERROR_NOT_FOUND) {
		return ddInjectorCrashEvent{}, fmt.Errorf("ProcessName: %w", err)
	}

	return ddInjectorCrashEvent{
		ProcessName: processName,
		ProcessID:   processID,
		ExitStatus:  fmt.Sprintf("0x%08x", exitStatus),
		ElapsedMs:   elapsedMs,
		Phase:       phase,
	}, nil
}

func tdhUint32Property(record *etw.DDEventRecord, name string) (uint32, error) {
	value, err := tdhEventProperty(record, name)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if len(value) != 4 {
		return 0, fmt.Errorf("%s has size %d, expected 4", name, len(value))
	}
	return binary.LittleEndian.Uint32(value), nil
}

func tdhInt64Property(record *etw.DDEventRecord, name string) (int64, error) {
	value, err := tdhEventProperty(record, name)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if len(value) != 8 {
		return 0, fmt.Errorf("%s has size %d, expected 8", name, len(value))
	}
	return int64(binary.LittleEndian.Uint64(value)), nil
}

func tdhEventName(record *etw.DDEventRecord) (string, error) {
	var size uint32
	status, _, _ := tdhGetEventInformationProc.Call(
		uintptr(unsafe.Pointer(record)),
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	if windows.Errno(status) != windows.ERROR_INSUFFICIENT_BUFFER {
		return "", windows.Errno(status)
	}

	buffer := make([]byte, size)
	status, _, _ = tdhGetEventInformationProc.Call(
		uintptr(unsafe.Pointer(record)),
		0,
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if windows.Errno(status) != windows.ERROR_SUCCESS {
		return "", windows.Errno(status)
	}
	if len(buffer) < traceEventInfoEventNameIndex+4 {
		return "", errors.New("TRACE_EVENT_INFO is too short")
	}

	eventNameOffset := binary.LittleEndian.Uint32(buffer[traceEventInfoEventNameIndex:])
	if eventNameOffset == 0 || int(eventNameOffset) >= len(buffer) || eventNameOffset%2 != 0 {
		return "", fmt.Errorf("invalid event name offset %d", eventNameOffset)
	}
	eventNameData := buffer[eventNameOffset:]
	eventNameUTF16 := unsafe.Slice((*uint16)(unsafe.Pointer(&eventNameData[0])), len(eventNameData)/2)
	return windows.UTF16ToString(eventNameUTF16), nil
}

func tdhEventProperty(record *etw.DDEventRecord, name string) ([]byte, error) {
	propertyName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	defer runtime.KeepAlive(propertyName)
	descriptor := tdhPropertyDataDescriptor{
		propertyName: uint64(uintptr(unsafe.Pointer(propertyName))),
		arrayIndex:   ^uint32(0),
	}

	var size uint32
	status, _, _ := tdhGetPropertySizeProc.Call(
		uintptr(unsafe.Pointer(record)),
		0,
		0,
		1,
		uintptr(unsafe.Pointer(&descriptor)),
		uintptr(unsafe.Pointer(&size)),
	)
	if windows.Errno(status) != windows.ERROR_SUCCESS {
		return nil, windows.Errno(status)
	}
	if size == 0 {
		return []byte{}, nil
	}

	buffer := make([]byte, size)
	status, _, _ = tdhGetPropertyProc.Call(
		uintptr(unsafe.Pointer(record)),
		0,
		0,
		1,
		uintptr(unsafe.Pointer(&descriptor)),
		uintptr(size),
		uintptr(unsafe.Pointer(&buffer[0])),
	)
	if windows.Errno(status) != windows.ERROR_SUCCESS {
		return nil, windows.Errno(status)
	}
	return buffer, nil
}
