// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"path"
	"runtime"
	"strconv"
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

// parseKlogHeaderCaller extracts file basename and line from a klog header:
// "Lmmdd hh:mm:ss.uuuuuu threadid file:line"
func parseKlogHeaderCaller(header string) (string, int, bool) {
	lastSpace := strings.LastIndexByte(header, ' ')
	if lastSpace < 0 {
		return "", 0, false
	}
	fileLine := header[lastSpace+1:]
	colon := strings.LastIndexByte(fileLine, ':')
	if colon <= 0 || colon == len(fileLine)-1 {
		return "", 0, false
	}
	line, err := strconv.Atoi(fileLine[colon+1:])
	if err != nil || line <= 0 {
		return "", 0, false
	}
	return fileLine[:colon], line, true
}

func resolveKlogFrameDepth(next func() (runtime.Frame, bool), headerFile string, headerLine int, hasHeaderCaller bool, fallback int) int {
	seenKlog := false
	firstNonKlogDepth := -1
	depth := 0
	for {
		frame, more := next()
		if depth > 0 {
			if isKlogFrame(frame.Function) {
				seenKlog = true
			} else if seenKlog {
				if firstNonKlogDepth < 0 {
					// depth 0 is Write (stackDepth 1); frame at index depth needs depth + 1.
					firstNonKlogDepth = depth + 1
					if !hasHeaderCaller {
						return firstNonKlogDepth
					}
				}
				if path.Base(frame.File) == headerFile && frame.Line == headerLine {
					return depth + 1
				}
			}
		}
		if !more {
			break
		}
		depth++
	}
	if firstNonKlogDepth > 0 {
		return firstNonKlogDepth
	}
	return fallback
}

func (l KlogRedirectLogger) resolveStackDepth(headerFile string, headerLine int, hasHeaderCaller bool) int {
	const maxFrames = 32
	var pcs [maxFrames]uintptr
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return l.stackDepth
	}
	frames := runtime.CallersFrames(pcs[:n])
	return resolveKlogFrameDepth(frames.Next, headerFile, headerLine, hasHeaderCaller, l.stackDepth)
}

func (l KlogRedirectLogger) Write(b []byte) (int, error) {
	// klog log lines have the following format:
	//     Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...
	// so we parse L to decide in which level to log, and we try to find
	// the ']' character, to ignore anything up to that point, as we don't
	// care about the header outside of the log level.

	var level LogLevel
	switch b[0] {
	case 'W':
		level = WarnLvl
	case 'E':
		level = ErrorLvl
	case 'F':
		level = CriticalLvl
	default:
		level = InfoLvl
	}

	if !ShouldLog(level) {
		return 0, nil
	}

	msg := string(b)
	var headerFile string
	var headerLine int
	var hasHeaderCaller bool

	i := strings.IndexByte(msg, ']')
	if i >= 0 {
		headerFile, headerLine, hasHeaderCaller = parseKlogHeaderCaller(msg[:i])
		// if we find a ']', we ignore anything 2 positions from it
		// (itself, plus a blank space)
		msg = msg[i+2:]
	}

	depth := l.resolveStackDepth(headerFile, headerLine, hasHeaderCaller)

	switch level {
	case WarnLvl:
		_ = WarnStackDepth(depth, msg)
	case ErrorLvl:
		_ = ErrorStackDepth(depth, msg)
	case CriticalLvl:
		_ = CriticalStackDepth(depth, msg)
	default:
		InfoStackDepth(depth, msg)
	}

	return 0, nil
}
