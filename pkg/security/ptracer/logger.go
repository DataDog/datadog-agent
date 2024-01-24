// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

import golog "log"

// Logger defines a logger
type Logger struct {
	Verbose bool
}

// Errorf print the error
func (l *Logger) Errorf(fmt string, args ...any) {
	golog.Printf(fmt, args...)
}

// Debugf print if verbose
func (l *Logger) Debugf(fmt string, args ...any) {
	if l.Verbose {
		golog.Printf(fmt, args...)
	}
}
