// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package agentcrashdetectimpl

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/time/rate"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	etw "github.com/DataDog/datadog-agent/comp/etw/def"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ddInjectorCrashEventType    = "ddinjector-crash"
	ddInjectorProviderGUID      = "{9933a039-281b-4342-a4e0-7109c8d3f22c}"
	ddInjectorETWSessionName    = "Datadog DDInjector crash telemetry"
	ddInjectorETWCrashKeyword   = uint64(0x40)
	ddInjectorCrashQueueSize    = 64
	ddInjectorCrashEventsPerMin = 10
)

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
	// ETW may deliver keyword-zero events even when MatchAnyKeyword is set.
	// TRACE_KEYWORD_CRASH is otherwise exclusive to CrashAttribution_Event.
	if record.EventHeader.EventDescriptor.Keyword&ddInjectorETWCrashKeyword == 0 {
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
	data := etwimpl.GetUserData(record)
	return decodeDDInjectorCrashUserData(data.Bytes(0, data.Length()))
}
