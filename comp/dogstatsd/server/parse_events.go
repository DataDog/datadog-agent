// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

type eventPriority int

const (
	priorityNormal eventPriority = iota
	priorityLow
)

type alertType int

const (
	alertTypeSuccess alertType = iota
	alertTypeInfo
	alertTypeWarning
	alertTypeError
)

type dogstatsdEvent struct {
	title          string
	text           string
	timestamp      int64
	hostname       string
	aggregationKey string
	priority       eventPriority
	sourceType     string
	alertType      alertType
	tags           []string
	// containerID represents the container ID of the sender (optional).
	containerID []byte
}

type eventHeader struct {
	titleLength int
	textLength  int
}

var (
	eventTimestampPrefix      = []byte("d:")
	eventHostnamePrefix       = []byte("h:")
	eventAggregationKeyPrefix = []byte("k:")
	eventPriorityPrefix       = []byte("p:")
	eventSourceTypePrefix     = []byte("s:")
	eventAlertTypePrefix      = []byte("t:")
	eventTagsPrefix           = []byte("#")

	eventPriorityLow    = []byte("low")
	eventPriorityNormal = []byte("normal")

	eventAlertTypeError   = []byte("error")
	eventAlertTypeWarning = []byte("warning")
	eventAlertTypeInfo    = []byte("info")
	eventAlertTypeSuccess = []byte("success")
)

// splitHeaderEvent splits the event and the
func splitHeaderEvent(message []byte) ([]byte, []byte, error) {
	panic("not called")
}

func parseHeader(rawHeader []byte) (eventHeader, error) {
	panic("not called")
}

func cleanEventText(text []byte) []byte {
	panic("not called")
}

func parseEventTimestamp(rawTimestamp []byte) (int64, error) {
	panic("not called")
}

func parseEventPriority(rawPriority []byte) (eventPriority, error) {
	panic("not called")
}

func parseEventAlertType(rawAlertType []byte) (alertType, error) {
	panic("not called")
}

func (p *parser) applyEventOptionalField(event dogstatsdEvent, optionalField []byte) (dogstatsdEvent, error) {
	panic("not called")
}

func (p *parser) parseEvent(message []byte) (dogstatsdEvent, error) {
	panic("not called")
}
