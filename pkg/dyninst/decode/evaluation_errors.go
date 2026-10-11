// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package decode

import (
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// EvaluationError describes an expression of a probe that couldn't be
// evaluated.
type EvaluationError struct {
	// Expression is the DSL of the expression.
	Expression string
	// Message says what went wrong.
	Message string
}

// evaluationErrorsMessage is a snapshot that carries nothing but evaluation
// errors.
type evaluationErrorsMessage struct {
	Service     string           `json:"service"`
	DDSource    ddDebuggerSource `json:"ddsource"`
	Logger      logger           `json:"logger"`
	Debugger    debuggerData     `json:"debugger"`
	Timestamp   int              `json:"timestamp"`
	Message     string           `json:"message"`
	ProcessTags string           `json:"process_tags,omitempty"`
}

// EncodeEvaluationErrors encodes a snapshot that reports evaluation errors of a
// probe. It is meant for probes that don't produce snapshots of their own,
// such as metric probes, so that the errors still show up with the probe.
func EncodeEvaluationErrors(
	probe ir.ProbeDefinition,
	service string,
	processTags string,
	timestamp time.Time,
	message string,
	errs []EvaluationError,
) ([]byte, error) {
	location, err := probeLocation(probe)
	if err != nil {
		return nil, err
	}
	ts := int(timestamp.UnixMilli())
	m := evaluationErrorsMessage{
		Service: service,
		Logger: logger{
			Method:  location.Method,
			Version: probe.GetVersion(),
		},
		Debugger: debuggerData{
			Snapshot: snapshotData{
				ID:        uuid.New(),
				Timestamp: ts,
				Language:  "go",
				Probe: probeData{
					ID:       probe.GetID(),
					Location: location,
				},
				EvaluationErrors: make([]evaluationError, 0, len(errs)),
			},
			Type: payloadTypeSnapshot,
		},
		Timestamp:   ts,
		Message:     message,
		ProcessTags: processTags,
	}
	for _, e := range errs {
		m.Debugger.Snapshot.EvaluationErrors = append(
			m.Debugger.Snapshot.EvaluationErrors,
			evaluationError(e),
		)
	}
	return json.Marshal(m)
}
