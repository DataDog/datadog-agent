// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	wildcard = "*"
	depth    = 4
)

// used to extract package.struct.func from the caller
var re = regexp.MustCompile(`[^\.]*\/([^\.]*)\.\(?\*?([^\.\)]*)\)?\.(.*)$`)

// PatternLogger is a wrapper for the agent logger that add a level of filtering to trace log level
type PatternLogger struct {
	sync.RWMutex

	patterns []string
	nodes    [][]string
}

func (l *PatternLogger) match(els []string) bool {
LOOP:
	for _, pattern := range l.nodes {
		for i, node := range pattern {
			if node == wildcard {
				continue
			}

			if i >= len(els) {
				break
			}

			if node != els[i] {
				continue LOOP
			}
		}

		return true
	}

	return false
}

func (l *PatternLogger) trace(format string, params ...interface{}) {
	pc, _, _, ok := runtime.Caller(3)
	if !ok {
		return
	}
	details := runtime.FuncForPC(pc)
	if details == nil {
		return
	}

	els := re.FindStringSubmatch(details.Name())
	if len(els) != 4 {
		return
	}

	l.RLock()
	active := l.match(els[1:])
	l.RUnlock()

	if active {
		log.TraceStackDepth(depth, fmt.Sprintf(format, params...))
	}
}

// Trace is used to print a trace level log
func (l *PatternLogger) Trace(v interface{}) {
	if logLevel, err := log.GetLogLevel(); err != nil || logLevel != seelog.TraceLvl {
		return
	}

	l.trace("%v", v)
}

// Tracef is used to print a trace level log
func (l *PatternLogger) Tracef(format string, params ...interface{}) {
	if logLevel, err := log.GetLogLevel(); err != nil || logLevel != seelog.TraceLvl {
		return
	}

	l.trace(format, params...)
}

// Debugf is used to print a trace level log
func (l *PatternLogger) Debugf(format string, params ...interface{}) {
	log.Debugf(format, params...)
}

// Errorf is used to print an error
func (l *PatternLogger) Errorf(format string, params ...interface{}) {
	_ = log.Errorf(format, params...)
}

// Warnf is used to print a warn
func (l *PatternLogger) Warnf(format string, params ...interface{}) {
	log.Warnf(format, params...)
}

// Infof is used to print an error
func (l *PatternLogger) Infof(format string, params ...interface{}) {
	log.Infof(format, params...)
}

// AddPatterns add new patterns
func (l *PatternLogger) AddPatterns(patterns ...string) []string {
	l.Lock()
	prev := l.patterns

	for _, pattern := range patterns {
		l.patterns = append(l.patterns, pattern)
		l.nodes = append(l.nodes, strings.Split(pattern, "."))
	}
	l.Unlock()

	return prev
}

// SetPatterns set patterns
func (l *PatternLogger) SetPatterns(patterns ...string) []string {
	l.Lock()
	prev := l.patterns

	l.nodes = [][]string{}
	for _, pattern := range patterns {
		l.nodes = append(l.nodes, strings.Split(pattern, "."))
	}
	l.Unlock()

	return prev
}

// DefaultLogger default logger of this package
var DefaultLogger *PatternLogger

// Debugf is used to print a trace level log
func Debugf(format string, params ...interface{}) {
	DefaultLogger.Debugf(format, params...)
}

// Errorf is used to print an error
func Errorf(format string, params ...interface{}) {
	DefaultLogger.Errorf(format, params...)
}

// Warnf is used to print a warn
func Warnf(format string, params ...interface{}) {
	DefaultLogger.Warnf(format, params...)
}

// Infof is used to print an error
func Infof(format string, params ...interface{}) {
	DefaultLogger.Infof(format, params...)
}

// Tracef is used to print an trace
func Tracef(format string, params ...interface{}) {
	DefaultLogger.Tracef(format, params...)
}

// Trace is used to print an trace
func Trace(v interface{}) {
	DefaultLogger.Trace(v)
}

// AddPatterns add patterns
func AddPatterns(patterns ...string) []string {
	return DefaultLogger.AddPatterns(patterns...)
}

// SetPatterns set patterns
func SetPatterns(patterns ...string) []string {
	return DefaultLogger.SetPatterns(patterns...)
}

func init() {
	DefaultLogger = &PatternLogger{}
}
