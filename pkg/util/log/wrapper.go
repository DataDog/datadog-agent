// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

// Wrapper wraps all the logger function on a struct. This is meant to be used by the comp/core/log component to expose
// the logger functionnality to components. This should only be use by the log component.
type Wrapper struct {
	stackDepth int
}

// NewWrapper returns a new Wrapper. This should only be use by the log component.
func NewWrapper(stackDepth int) *Wrapper {
	return &Wrapper{stackDepth: stackDepth}
}

// Until the log migration to component is done, we use *StackDepth to pkglog. The log component add 1 layer to the call
// stack and *StackDepth add another.
//
// We check the current log level to avoid calling Sprintf when it's not needed (Sprintf from Tracef uses a lot a CPU)

// Trace implements Component#Trace.
func (l *Wrapper) Trace(v ...interface{}) { TraceStackDepth(l.stackDepth, v...) }

// Tracef implements Component#Tracef.
func (l *Wrapper) Tracef(format string, params ...interface{}) {
	TracefStackDepth(l.stackDepth, format, params...)
}

// Debug implements Component#Debug.
func (l *Wrapper) Debug(v ...interface{}) { DebugStackDepth(l.stackDepth, v...) }

// Debugf implements Component#Debugf.
func (l *Wrapper) Debugf(format string, params ...interface{}) {
	DebugfStackDepth(l.stackDepth, format, params...)
}

// Info implements Component#Info.
func (l *Wrapper) Info(v ...interface{}) { InfoStackDepth(l.stackDepth, v...) }

// Infof implements Component#Infof.
func (l *Wrapper) Infof(format string, params ...interface{}) {
	InfofStackDepth(l.stackDepth, format, params...)
}

// Warn implements Component#Warn.
func (l *Wrapper) Warn(v ...interface{}) error { return WarnStackDepth(l.stackDepth, v...) }

// Warnf implements Component#Warnf.
func (l *Wrapper) Warnf(format string, params ...interface{}) error {
	return WarnfStackDepth(l.stackDepth, format, params...)
}

// Error implements Component#Error.
func (l *Wrapper) Error(v ...interface{}) error { return ErrorStackDepth(l.stackDepth, v...) }

// Errorf implements Component#Errorf.
func (l *Wrapper) Errorf(format string, params ...interface{}) error {
	return ErrorfStackDepth(l.stackDepth, format, params...)
}

// Critical implements Component#Critical.
func (l *Wrapper) Critical(v ...interface{}) error {
	return CriticalStackDepth(l.stackDepth, v...)
}

// Criticalf implements Component#Criticalf.
func (l *Wrapper) Criticalf(format string, params ...interface{}) error {
	return CriticalfStackDepth(l.stackDepth, format, params...)
}

// Flush implements Component#Flush.
func (l *Wrapper) Flush() {
	Flush()
}
