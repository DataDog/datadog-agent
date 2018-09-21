// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import (
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
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
	parser.StatusEmergency: SevEmergency,
	parser.StatusAlert:     SevAlert,
	parser.StatusCritical:  SevCritical,
	parser.StatusError:     SevError,
	parser.StatusWarning:   SevWarning,
	parser.StatusNotice:    SevNotice,
	parser.StatusInfo:      SevInfo,
	parser.StatusDebug:     SevDebug,
}

// StatusToSeverity transforms a severity into a status.
func StatusToSeverity(status string) []byte {
	if sev, exists := statusSeverityMapping[status]; exists {
		return sev
	}
	return SevInfo
}
