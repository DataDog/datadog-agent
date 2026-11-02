// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"bytes"
	"errors"
	"fmt"
	"math/bits"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventPriority is an event's wire priority.
type EventPriority int

const (
	PriorityNormal EventPriority = iota
	PriorityLow
)

// AlertType is an event's wire alert level.
type AlertType int

const (
	AlertSuccess AlertType = iota
	AlertInfo
	AlertWarning
	AlertError
)

// Event contains parsed event fields before server enrichment.
type Event struct {
	Title          string
	Text           string
	Timestamp      int64
	Hostname       string
	AggregationKey string
	Priority       EventPriority
	SourceType     string
	AlertType      AlertType
	Tags           []string
	// localData is used for Origin Detection
	LocalData origindetection.LocalData
	// externalData is used for Origin Detection
	ExternalData origindetection.ExternalData
	// cardinality is used for Origin Detection
	Cardinality string
}

type eventHeader struct {
	titleLength uint
	textLength  uint
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
	sepIndex := bytes.Index(message, colonSeparator)
	if sepIndex == -1 {
		return nil, nil, fmt.Errorf("invalid event: %q", message)
	}
	return message[:sepIndex], message[sepIndex+1:], nil
}

func parseHeader(rawHeader []byte) (eventHeader, error) {
	// Basic sanity check on the header (minimum length)
	if len(rawHeader) < 7 {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}

	// Extract out the raw title and text lengths
	rawLengths := rawHeader[3 : len(rawHeader)-1]
	sepIndex := bytes.Index(rawLengths, commaSeparator)
	if sepIndex == -1 {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}

	rawTitleLength := rawLengths[:sepIndex]
	rawTextLength := rawLengths[sepIndex+1:]

	// Convert title length to workable type and do a basic validity check on value
	titleLength, err := parseInt(rawTitleLength)
	if err != nil || titleLength < 0 {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}

	// Ensure that title isn't empty
	if titleLength == 0 {
		return eventHeader{}, errors.New("invalid event: empty title")
	}

	// Convert text length to workable type and do a basic validity check on value
	textLength, err := parseInt(rawTextLength)
	if err != nil || textLength < 0 {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}

	return eventHeader{
		titleLength: uint(titleLength),
		textLength:  uint(textLength),
	}, nil
}

func cleanEventText(text []byte) []byte {
	return bytes.Replace(text, []byte("\\n"), []byte("\n"), -1)
}

func parseEventTimestamp(rawTimestamp []byte) (int64, error) {
	return parseInt64(rawTimestamp)
}

func parseEventPriority(rawPriority []byte) (EventPriority, error) {
	switch {
	case bytes.Equal(rawPriority, eventPriorityNormal):
		return PriorityNormal, nil
	case bytes.Equal(rawPriority, eventPriorityLow):
		return PriorityLow, nil
	}
	return PriorityNormal, fmt.Errorf("invalid event priority: %q", rawPriority)
}

func parseEventAlertType(rawAlertType []byte) (AlertType, error) {
	switch {
	case bytes.Equal(rawAlertType, eventAlertTypeSuccess):
		return AlertSuccess, nil
	case bytes.Equal(rawAlertType, eventAlertTypeInfo):
		return AlertInfo, nil
	case bytes.Equal(rawAlertType, eventAlertTypeWarning):
		return AlertWarning, nil
	case bytes.Equal(rawAlertType, eventAlertTypeError):
		return AlertError, nil
	}
	return AlertInfo, fmt.Errorf("invalid alert type: %q", rawAlertType)
}

func (p *Parser) applyEventOptionalField(event Event, optionalField []byte) (Event, error) {
	newEvent := event
	var err error
	switch {
	case bytes.HasPrefix(optionalField, eventTimestampPrefix):
		newEvent.Timestamp, err = parseEventTimestamp(optionalField[len(eventTimestampPrefix):])
	case bytes.HasPrefix(optionalField, eventHostnamePrefix):
		newEvent.Hostname = string(optionalField[len(eventHostnamePrefix):])
	case bytes.HasPrefix(optionalField, eventAggregationKeyPrefix):
		newEvent.AggregationKey = string(optionalField[len(eventAggregationKeyPrefix):])
	case bytes.HasPrefix(optionalField, eventPriorityPrefix):
		newEvent.Priority, err = parseEventPriority(optionalField[len(eventPriorityPrefix):])
	case bytes.HasPrefix(optionalField, eventSourceTypePrefix):
		newEvent.SourceType = string(optionalField[len(eventSourceTypePrefix):])
	case bytes.HasPrefix(optionalField, eventAlertTypePrefix):
		newEvent.AlertType, err = parseEventAlertType(optionalField[len(eventAlertTypePrefix):])
	case bytes.HasPrefix(optionalField, eventTagsPrefix):
		newEvent.Tags = p.parseTags(optionalField[len(eventTagsPrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, localDataPrefix):
		newEvent.LocalData = p.parseLocalData(optionalField[len(localDataPrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, externalDataPrefix):
		newEvent.ExternalData = p.parseExternalData(optionalField[len(externalDataPrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, cardinalityPrefix):
		newEvent.Cardinality = string(optionalField[len(cardinalityPrefix):])
	}
	if err != nil {
		return event, err
	}
	return newEvent, nil
}

// ParseEvent parses an event, ignoring malformed optional fields as before.
func (p *Parser) ParseEvent(message []byte) (Event, error) {
	rawHeader, rawEvent, err := splitHeaderEvent(message)
	if err != nil {
		return Event{}, err
	}

	header, err := parseHeader(rawHeader)
	if err != nil {
		return Event{}, err
	}

	textStart, overflow := bits.Add(header.titleLength, 1, 0)
	if overflow > 0 {
		return Event{}, errors.New("invalid event")
	}
	contentLength, overflow := bits.Add(textStart, header.textLength, 0)
	if overflow > 0 {
		return Event{}, errors.New("invalid event")
	}

	if uint(len(rawEvent)) < contentLength {
		return Event{}, errors.New("invalid event")
	}

	title := cleanEventText(rawEvent[:header.titleLength])
	text := cleanEventText(rawEvent[textStart:contentLength])

	event := Event{
		Title:     string(title),
		Text:      string(text),
		Priority:  PriorityNormal,
		AlertType: AlertInfo,
	}

	if uint(len(rawEvent)) == contentLength {
		return event, nil
	}

	optionalFields := rawEvent[contentLength:]
	var optionalField []byte
	for optionalFields != nil {
		optionalField, optionalFields = nextField(optionalFields)
		event, err = p.applyEventOptionalField(event, optionalField)
		if err != nil {
			log.Warnf("invalid event optional field: %v", err)
		}
	}
	return event, nil
}
