// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

// trapLogger is a GoSNMP logger interface implementation.
type trapLogger struct {
	gosnmp.Logger
	logger log.Component
}

// NOTE: GoSNMP logs show the full content of decoded trap packets. Logging as DEBUG would be too noisy.
func (logger *trapLogger) Print(v ...interface{}) {
	logger.logger.Trace(v...)
}
func (logger *trapLogger) Printf(format string, v ...interface{}) {
	logger.logger.Tracef(format, v...)
}
