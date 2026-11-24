// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"runtime"
	"strings"
)

// KlogRedirectLogger is used to redirect klog logs to datadog logs. klog is
// client-go's logger, logging to STDERR by default, which makes all severities
// into ERROR, along with the formatting just being off. To make the
// conversion, we set a KlogRedirectLogger as klog's output, and parse the severity
// and log message out of every log line.
// NOTE: on klog v2 this parsing is no longer necessary, as it allows us to use
// kSetLogger() instead of kSetOutputBySeverity(). unfortunately we
// still have some dependencies stuck on v1, so we keep the parsing.
type KlogRedirectLogger struct {
	stackDepth int
}

// NewKlogRedirectLogger creates a new KlogRedirectLogger with provided stack depth
func NewKlogRedirectLogger(stackDepth int) KlogRedirectLogger {
	return KlogRedirectLogger{
		stackDepth: stackDepth,
	}
}

func isKlogFrame(fn string) bool {
	return strings.HasPrefix(fn, "k8s.io/klog") ||
		strings.HasPrefix(fn, "github.com/go-logr/logr.")
}

func stackDepthFromFunctionNames(fns []string, fallback int) int {
	seenKlog := false
	for depth, fn := range fns {
		if depth == 0 {
			continue
		}
		if isKlogFrame(fn) {
			seenKlog = true
		} else if seenKlog {
			// depth 0 is Write (stackDepth 1); frame at index depth needs depth + 1.
			return depth + 1
		}
	}
	return fallback
}

func (l KlogRedirectLogger) resolveStackDepth() int {
	const maxFrames = 32
	var pcs [maxFrames]uintptr
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return l.stackDepth
	}
	frames := runtime.CallersFrames(pcs[:n])
	fns := make([]string, 0, n)
	for {
		frame, more := frames.Next()
		fns = append(fns, frame.Function)
		if !more {
			break
		}
	}
	return stackDepthFromFunctionNames(fns, l.stackDepth)
}

func (l KlogRedirectLogger) Write(b []byte) (int, error) {
	// klog log lines have the following format:
	//     Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...
	// so we parse L to decide in which level to log, and we try to find
	// the ']' character, to ignore anything up to that point, as we don't
	// care about the header outside of the log level.

	msg := string(b)

	i := strings.IndexByte(msg, ']')
	if i >= 0 {
		// if we find a ']', we ignore anything 2 positions from it
		// (itself, plus a blank space)
		msg = msg[i+2:]
	}

	depth := l.resolveStackDepth()

	switch b[0] {
	case 'I':
		InfoStackDepth(depth, msg)
	case 'W':
		_ = WarnStackDepth(depth, msg)
	case 'E':
		_ = ErrorStackDepth(depth, msg)
	case 'F':
		_ = CriticalStackDepth(depth, msg)
	default:
		InfoStackDepth(depth, msg)
	}

	return 0, nil
}
