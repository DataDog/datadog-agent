// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package snmplog provides a GoSNMP logger that wraps our logger.
package snmplog

import (
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

// SNMPLogger is a GoSNMP logger interface implementation.
type SNMPLogger struct {
	gosnmp.LoggerInterface
	logger log.Component
}

var _ gosnmp.LoggerInterface = (*SNMPLogger)(nil)

// New creates a new SNMPLogger
func New(logger log.Component) *SNMPLogger {
	return &SNMPLogger{
		logger: logger,
	}
}

// Print implements gosnmp.LoggerInterface#Print
func (logger *SNMPLogger) Print(v ...interface{}) {
	// NOTE: GoSNMP logs show the full content of decoded trap packets. Logging as DEBUG would be too noisy.
	logger.logger.Trace(v...)
}

// Printf implements gosnmp.LoggerInterface#Printf
func (logger *SNMPLogger) Printf(format string, v ...interface{}) {
	logger.logger.Tracef(format, v...)
}
