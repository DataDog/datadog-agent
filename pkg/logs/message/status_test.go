// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
)

func TestStatusToSeverity(t *testing.T) {
	severityLevels := [][]byte{SevEmergency, SevAlert, SevCritical, SevError, SevWarning, SevNotice, SevInfo, SevDebug}
	statusValues := []string{
		parser.StatusEmergency,
		parser.StatusAlert,
		parser.StatusCritical,
		parser.StatusError,
		parser.StatusWarning,
		parser.StatusNotice,
		parser.StatusInfo,
		parser.StatusDebug,
	}

	// ensure 1:1 mapping
	for i, status := range statusValues {
		assert.Equal(t, severityLevels[i], StatusToSeverity(status))
	}

	// default value should be "info"
	assert.Equal(t, 0, bytes.Compare(SevInfo, StatusToSeverity("foo")))
}
