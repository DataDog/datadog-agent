// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logging

import (
	"fmt"

	"go.uber.org/fx/fxevent"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil/lifecycle"
)

// LifecycleLogger implements fxevent.Logger to record the duration of the Fx constructors and
// lifecycle hooks in a lifecycle.Recorder, then forwards the events to the next logger.
type LifecycleLogger struct {
	recorder *lifecycle.Recorder

	next fxevent.Logger
}

var _ fxevent.Logger = (*LifecycleLogger)(nil)

func withLifecycleRecorder(next fxevent.Logger, recorder *lifecycle.Recorder) *LifecycleLogger {
	return &LifecycleLogger{recorder: recorder, next: next}
}

// Next returns the logger the events are forwarded to.
func (l *LifecycleLogger) Next() fxevent.Logger {
	return l.next
}

// LogEvent implements the fxevent.Logger interface.
func (l *LifecycleLogger) LogEvent(event fxevent.Event) {
	switch e := event.(type) {
	case *fxevent.Run:
		l.recorder.RecordComponent(lifecycle.StageConstruct, runName(e), e.Runtime)
	case *fxevent.OnStartExecuted:
		l.recorder.RecordComponent(lifecycle.StageOnStart, extractShortPathFromFullPath(e.FunctionName), e.Runtime)
	case *fxevent.OnStopExecuting:
		l.recorder.ComponentStopping(extractShortPathFromFullPath(e.FunctionName))
	case *fxevent.OnStopExecuted:
		l.recorder.RecordComponent(lifecycle.StageOnStop, extractShortPathFromFullPath(e.FunctionName), e.Runtime)
	}

	l.next.LogEvent(event)
}

func runName(e *fxevent.Run) string {
	name := extractShortPathFromFullPath(e.Name)
	if name == "reflect.makeFuncStub()" {
		// fallback to the module to at least know where this comes from
		name = fmt.Sprintf("%s(%s)", e.Kind, e.ModuleName) // eg. "provide(comp/core/ipc)"
	}
	return name
}

// TracingLogger returns the FxTracingLogger in the chain of loggers starting at logger, if any.
func TracingLogger(logger fxevent.Logger) (*FxTracingLogger, bool) {
	if l, ok := logger.(*LifecycleLogger); ok {
		logger = l.Next()
	}
	tracer, ok := logger.(*FxTracingLogger)
	return tracer, ok
}
