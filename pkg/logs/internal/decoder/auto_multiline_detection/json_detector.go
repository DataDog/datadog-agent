// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import "regexp"

var jsonRegexp = regexp.MustCompile(`^\s*\{\s*["}]`)

// JSONDetector is a heuristic to detect JSON messages.
type JSONDetector struct{}

// NewJSONDetector returns a new JSON detection heuristic.
func NewJSONDetector() *JSONDetector {
	return &JSONDetector{}
}

// ProcessAndContinue checks if a message is a JSON message.
// This implements the Herustic interface - so we should stop processing if we detect a JSON message by returning false.
func (j *JSONDetector) ProcessAndContinue(context *messageContext) bool {
	if jsonRegexp.Match(context.rawMessage) {
		context.label = noAggregate
		return false
	}
	return true
}
