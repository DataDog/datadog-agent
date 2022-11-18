// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// trapLogger is a GoSNMP logger interface implementation.
type trapLogger struct {
	gosnmp.Logger
}

// NOTE: GoSNMP logs show the full content of decoded trap packets. Logging as DEBUG would be too noisy.
func (logger *trapLogger) Print(v ...interface{}) {
	log.Trace(v...)
}
func (logger *trapLogger) Printf(format string, v ...interface{}) {
	log.Tracef(format, v...)
}
