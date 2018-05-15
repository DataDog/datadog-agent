// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import (
	"bytes"
)

// Syslog severity levels,
// for more information check https://en.wikipedia.org/wiki/Syslog#Severity_level.
var (
	SevEmergency = []byte("<40>")
	SevAlert     = []byte("<41>")
	SevCritical  = []byte("<42>")
	SevError     = []byte("<43>")
	SevWarning   = []byte("<44>")
	SevNotice    = []byte("<45>")
	SevInfo      = []byte("<46>")
	SevDebug     = []byte("<47>")
)

// Status values
const (
	StatusEmergency = "emerg"
	StatusAlert     = "alert"
	StatusCritical  = "critical"
	StatusError     = "error"
	StatusWarning   = "warning"
	StatusNotice    = "notice"
	StatusInfo      = "info"
	StatusDebug     = "debug"
)

// SeverityToStatus transforms a severity to a status.
func SeverityToStatus(severity []byte) string {
	switch {
	case bytes.Compare(severity, SevEmergency) == 0:
		return StatusEmergency
	case bytes.Compare(severity, SevAlert) == 0:
		return StatusAlert
	case bytes.Compare(severity, SevCritical) == 0:
		return StatusCritical
	case bytes.Compare(severity, SevError) == 0:
		return StatusError
	case bytes.Compare(severity, SevWarning) == 0:
		return StatusWarning
	case bytes.Compare(severity, SevNotice) == 0:
		return StatusNotice
	case bytes.Compare(severity, SevInfo) == 0:
		return StatusInfo
	case bytes.Compare(severity, SevDebug) == 0:
		return StatusDebug
	default:
		return StatusInfo
	}
}
