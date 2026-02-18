// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"bytes"
	"encoding/json"
	"io"
)

// JSONState represents the state of the JSON validator.
type JSONState int

const (
	// Incomplete indicates that the JSON validator is still processing the JSON message.
	Incomplete JSONState = iota
	// Complete indicates that the JSON validator has processed the entire JSON message and the JSON is valid.
	Complete
	// Invalid indicates that the JSON is invalid.
	Invalid
)

// IncrementalJSONValidator is a JSON validator that processes JSON messages incrementally.
type IncrementalJSONValidator struct {
	decoder  *json.Decoder
	writer   *bytes.Buffer
	objCount int
}

// NewIncrementalJSONValidator creates a new IncrementalJSONValidator.
func NewIncrementalJSONValidator() *IncrementalJSONValidator {
	buf := &bytes.Buffer{}
	return &IncrementalJSONValidator{
		decoder:  json.NewDecoder(buf),
		writer:   buf,
		objCount: 0,
	}
}

// Write writes a byte slice to the IncrementalJSONValidator.
func (d *IncrementalJSONValidator) Write(s []byte) JSONState {
	_, err := d.writer.Write(s)

	if err != nil {
		return Invalid
	}

	isValid := false
	for {
		t, err := d.decoder.Token()
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			break
		}
		if err != nil {
			return Invalid
		}
		isValid = true

		switch delim := t.(type) {
		case json.Delim:
			if delim.String() == "{" {
				d.objCount++
				break
			}
			if delim.String() == "}" {
				d.objCount--
				break
			}
			// If we're not in an object, we can't have a valid JSON message
			if d.objCount == 0 {
				isValid = false
			}
		}

	}
	if !isValid {
		return Invalid
	}

	if d.objCount <= 0 {
		return Complete
	}
	return Incomplete
}

// Reset resets the IncrementalJSONValidator.
func (d *IncrementalJSONValidator) Reset() {
	d.writer.Reset()
	d.decoder = json.NewDecoder(d.writer)
	d.objCount = 0
}
