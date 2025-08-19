// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

// Status values
const (
	StatusEmergency = "emergency"
	StatusAlert     = "alert"
	StatusCritical  = "critical"
	StatusError     = "error"
	StatusWarning   = "warn"
	StatusNotice    = "notice"
	StatusInfo      = "info"
	StatusDebug     = "debug"
)

// Syslog severity levels
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

// statusSeverityMapping represents the 1:1 mapping between statuses and severities.
var statusSeverityMapping = map[string][]byte{
	StatusEmergency: SevEmergency,
	StatusAlert:     SevAlert,
	StatusCritical:  SevCritical,
	StatusError:     SevError,
	StatusWarning:   SevWarning,
	StatusNotice:    SevNotice,
	StatusInfo:      SevInfo,
	StatusDebug:     SevDebug,
}

// StatusToSeverity transforms a severity into a status.
func StatusToSeverity(status string) []byte {
	if sev, exists := statusSeverityMapping[status]; exists {
		return sev
	}
	return SevInfo
}
