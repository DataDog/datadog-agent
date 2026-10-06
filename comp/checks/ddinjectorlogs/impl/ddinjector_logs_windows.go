// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ddinjectorlogsimpl

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/time/rate"

	ddinjectorlogs "github.com/DataDog/datadog-agent/comp/checks/ddinjectorlogs/def"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	compsysconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	etw "github.com/DataDog/datadog-agent/comp/etw/def"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
	winetw "github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
)

// Requires defines the dependencies for the DDInjector logs component.
type Requires struct {
	Config    compsysconfig.Component
	Atel      agenttelemetry.Component
	Etw       etw.Component
	Log       corelog.Component
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the DDInjector logs component.
type Provides struct {
	Comp ddinjectorlogs.Component
}

const (
	ddInjectorProviderGUID       = "{9933a039-281b-4342-a4e0-7109c8d3f22c}"
	ddInjectorETWSessionName     = "Datadog DDInjector logs"
	ddInjectorLogEventsPerMinute = 10
	// eventHeaderExtTypeEventSchemaTL is EVENT_HEADER_EXT_TYPE_EVENT_SCHEMA_TL: the extended data item
	// carrying the self-describing TraceLogging metadata of an event.
	eventHeaderExtTypeEventSchemaTL = 0x000B
)

// ddInjectorLog is the message forwarded for each watched event.
type ddInjectorLog struct {
	Event            string                 `json:"event"`
	Level            uint8                  `json:"level"`
	Keyword          uint64                 `json:"keyword"`
	Properties       map[string]interface{} `json:"properties"`
	DecodeError      string                 `json:"decode_error,omitempty"`
	EventsSuppressed uint64                 `json:"events_suppressed,omitempty"`
}

// watchedEvent describes a DDInjector TraceLogging event forwarded as a log.
// keyword and level must match the TraceLoggingWrite call so the session enables the event.
type watchedEvent struct {
	keyword   uint64
	level     etw.TraceLevel
	errorKind string
}

// watchedEvents maps TraceLogging event names to forward. All properties of a matching event are sent.
var watchedEvents = map[string]watchedEvent{
	"CrashAttribution_Event": {keyword: 0x40, level: etw.TRACE_LEVEL_WARNING, errorKind: "ddinjector_crash"},
}

// eventForwarder holds the per-event rate limiting state, so one noisy event cannot starve the others.
type eventForwarder struct {
	watchedEvent
	limiter    *rate.Limiter
	suppressed atomic.Uint64
}

type ddInjectorLogsListener struct {
	config compsysconfig.Component
	atel   agenttelemetry.Component
	etw    etw.Component
	log    corelog.Component

	providerGUID     windows.GUID
	forwarders       map[string]*eventForwarder
	decodeProperties func(*etw.DDEventRecord) (map[string]interface{}, error)
	decodeErrors     *utillog.Limit

	session   etw.Session
	traceDone chan struct{}
}

func newDDInjectorLogsListener(
	config compsysconfig.Component,
	atel agenttelemetry.Component,
	etwComponent etw.Component,
	log corelog.Component,
) *ddInjectorLogsListener {
	providerGUID, _ := windows.GUIDFromString(ddInjectorProviderGUID)
	forwarders := make(map[string]*eventForwarder, len(watchedEvents))
	for name, event := range watchedEvents {
		forwarders[name] = &eventForwarder{
			watchedEvent: event,
			limiter: rate.NewLimiter(
				rate.Every(time.Minute/ddInjectorLogEventsPerMinute),
				ddInjectorLogEventsPerMinute,
			),
		}
	}
	return &ddInjectorLogsListener{
		config:           config,
		atel:             atel,
		etw:              etwComponent,
		log:              log,
		providerGUID:     providerGUID,
		forwarders:       forwarders,
		decodeProperties: decodeEventProperties,
		decodeErrors:     utillog.NewLogLimit(1, 10*time.Minute),
	}
}

// NewComponent creates the DDInjector logs component.
func NewComponent(reqs Requires) Provides {
	listener := newDDInjectorLogsListener(reqs.Config, reqs.Atel, reqs.Etw, reqs.Log)
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: listener.start,
		OnStop:  listener.stop,
	})
	return Provides{Comp: listener}
}

func (l *ddInjectorLogsListener) start(_ context.Context) error {
	if !l.config.GetBool("windows_crash_detection.enabled") {
		return nil
	}

	session, err := l.etw.NewSession(ddInjectorETWSessionName, func(_ *etw.SessionConfiguration) {})
	if err != nil {
		l.log.Warnf("Could not create the DDInjector logs ETW session: %v", err)
		return nil
	}

	var keywords uint64
	level := etw.TRACE_LEVEL_CRITICAL
	for _, forwarder := range l.forwarders {
		keywords |= forwarder.keyword
		level = max(level, forwarder.level)
	}
	session.ConfigureProvider(l.providerGUID, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = level
		cfg.MatchAnyKeyword = keywords
	})
	if err = session.EnableProvider(l.providerGUID); err != nil {
		l.log.Warnf("Could not enable the DDInjector logs ETW provider: %v", err)
		_ = session.StopTracing()
		return nil
	}

	l.session = session
	l.traceDone = make(chan struct{})

	go l.runTrace()
	l.log.Info("DDInjector ETW logs forwarding is enabled")
	return nil
}

func (l *ddInjectorLogsListener) runTrace() {
	defer close(l.traceDone)
	if err := l.session.StartTracing(l.handleEvent); err != nil {
		l.log.Warnf("DDInjector logs ETW tracing stopped unexpectedly: %v", err)
	}
}

func (l *ddInjectorLogsListener) handleEvent(record *etw.DDEventRecord) {
	// The enabled keywords and level may let through events that are not watched; filter on the
	// name before paying for property decoding.
	name, ok := eventName(record)
	if !ok {
		return
	}
	forwarder, ok := l.forwarders[name]
	if !ok {
		return
	}
	if !forwarder.limiter.Allow() {
		forwarder.suppressed.Add(1)
		return
	}

	descriptor := record.EventHeader.EventDescriptor
	event := ddInjectorLog{
		Event:   name,
		Level:   descriptor.Level,
		Keyword: descriptor.Keyword,
	}
	properties, err := l.decodeProperties(record)
	if err != nil {
		// Forward what was decoded: a partial event is more useful than none.
		event.DecodeError = err.Error()
		l.logDecodeError(name, err)
	}
	event.Properties = properties
	event.EventsSuppressed = forwarder.suppressed.Swap(0)

	message, err := json.Marshal(event)
	if err != nil {
		l.log.Debugf("Could not marshal DDInjector %s log: %v", name, err)
		forwarder.suppressed.Add(event.EventsSuppressed + 1)
		return
	}
	if !l.atel.SubmitLog(agenttelemetry.Log{
		Message:    string(message),
		Level:      agenttelemetry.LogLevelError,
		TracerTime: time.Now().Unix(),
		Count:      1,
		ErrorKind:  forwarder.errorKind,
	}) {
		// Restore the prior suppressed count and include the event that could not be queued.
		forwarder.suppressed.Add(event.EventsSuppressed + 1)
	}
}

func (l *ddInjectorLogsListener) logDecodeError(name string, err error) {
	if l.decodeErrors.ShouldLog() {
		l.log.Warnf("Could not decode DDInjector ETW event %s: %v", name, err)
	}
}

func (l *ddInjectorLogsListener) stop(ctx context.Context) error {
	if l.session == nil {
		return nil
	}

	if err := l.session.StopTracing(); err != nil {
		l.log.Debugf("Could not stop the DDInjector logs ETW session: %v", err)
	}

	select {
	case <-l.traceDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// eventName returns the TraceLogging name of the event, read from its schema extended data item.
func eventName(record *etw.DDEventRecord) (string, bool) {
	if record.ExtendedData == nil {
		return "", false
	}
	items := unsafe.Slice(record.ExtendedData, record.ExtendedDataCount)
	for _, item := range items {
		if item.ExtType != eventHeaderExtTypeEventSchemaTL || item.DataPtr == nil {
			continue
		}
		name, err := traceLoggingEventName(unsafe.Slice(item.DataPtr, item.DataSize))
		return name, err == nil
	}
	return "", false
}

func decodeEventProperties(record *etw.DDEventRecord) (map[string]interface{}, error) {
	return winetw.EventRecordProperties(unsafe.Pointer(record))
}
