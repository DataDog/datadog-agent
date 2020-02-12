package dogstatsd

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	sepIndex := bytes.Index(message, colonSeparator)
	if sepIndex == -1 {
		return nil, nil, fmt.Errorf("invalid event: %q", message)
	}
	return message[:sepIndex], message[sepIndex+1:], nil
}

func parseHeader(rawHeader []byte) (eventHeader, error) {
	if len(rawHeader) < 7 {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}
	rawLengths := rawHeader[3 : len(rawHeader)-1]
	sepIndex := bytes.Index(rawLengths, commaSeparator)
	if sepIndex == -1 {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}
	rawTitleLength := rawLengths[:sepIndex]
	rawTextLength := rawLengths[sepIndex+1:]
	titleLength, err := parseInt64(rawTitleLength)
	if err != nil {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}
	textLength, err := parseInt64(rawTextLength)
	if err != nil {
		return eventHeader{}, fmt.Errorf("invalid event header: %q", rawHeader)
	}
	return eventHeader{
		titleLength: int(titleLength),
		textLength:  int(textLength),
	}, nil
}

func cleanEventText(text []byte) []byte {
	return bytes.Replace(text, []byte("\\n"), []byte("\n"), -1)
}

func parseEventTimestamp(rawTimestamp []byte) (int64, error) {
	return parseInt64(rawTimestamp)
}

func parseEventPriority(rawPriority []byte) (eventPriority, error) {
	switch {
	case bytes.Equal(rawPriority, eventPriorityNormal):
		return priorityNormal, nil
	case bytes.Equal(rawPriority, eventPriorityLow):
		return priorityLow, nil
	}
	return priorityNormal, fmt.Errorf("invalid event priority: %q", rawPriority)
}

func parseEventAlertType(rawAlertType []byte) (alertType, error) {
	switch {
	case bytes.Equal(rawAlertType, eventAlertTypeSuccess):
		return alertTypeSuccess, nil
	case bytes.Equal(rawAlertType, eventAlertTypeInfo):
		return alertTypeInfo, nil
	case bytes.Equal(rawAlertType, eventAlertTypeWarning):
		return alertTypeWarning, nil
	case bytes.Equal(rawAlertType, eventAlertTypeError):
		return alertTypeError, nil
	}
	return alertTypeInfo, fmt.Errorf("invalid alert type: %q", rawAlertType)
}

func (p *parser) applyEventOptionalField(event dogstatsdEvent, optionalField []byte) (dogstatsdEvent, error) {
	newEvent := event
	var err error
	switch {
	case bytes.HasPrefix(optionalField, eventTimestampPrefix):
		newEvent.timestamp, err = parseEventTimestamp(optionalField[len(eventTimestampPrefix):])
	case bytes.HasPrefix(optionalField, eventHostnamePrefix):
		newEvent.hostname = string(optionalField[len(eventHostnamePrefix):])
	case bytes.HasPrefix(optionalField, eventAggregationKeyPrefix):
		newEvent.aggregationKey = string(optionalField[len(eventAggregationKeyPrefix):])
	case bytes.HasPrefix(optionalField, eventPriorityPrefix):
		newEvent.priority, err = parseEventPriority(optionalField[len(eventPriorityPrefix):])
	case bytes.HasPrefix(optionalField, eventSourceTypePrefix):
		newEvent.sourceType = string(optionalField[len(eventSourceTypePrefix):])
	case bytes.HasPrefix(optionalField, eventAlertTypePrefix):
		newEvent.alertType, err = parseEventAlertType(optionalField[len(eventAlertTypePrefix):])
	case bytes.HasPrefix(optionalField, eventTagsPrefix):
		newEvent.tags = p.parseTags(optionalField[len(eventTagsPrefix):])
	}
	if err != nil {
		return event, err
	}
	return newEvent, nil
}

func (p *parser) parseEvent(message []byte) (dogstatsdEvent, error) {
	rawHeader, rawEvent, err := splitHeaderEvent(message)
	if err != nil {
		return dogstatsdEvent{}, err
	}
	header, err := parseHeader(rawHeader)
	if err != nil {
		return dogstatsdEvent{}, err
	}
	if len(rawEvent) < header.textLength+header.titleLength+1 {
		return dogstatsdEvent{}, fmt.Errorf("invalid event")
	}
	if header.titleLength == 0 || header.textLength == 0 {
		return dogstatsdEvent{}, fmt.Errorf("invalid event: empty title or text")
	}
	title := cleanEventText(rawEvent[:header.titleLength])
	text := cleanEventText(rawEvent[header.titleLength+1 : header.titleLength+1+header.textLength])

	event := dogstatsdEvent{
		title:     string(title),
		text:      string(text),
		priority:  priorityNormal,
		alertType: alertTypeInfo,
	}

	if len(rawEvent) == header.textLength+header.titleLength+1 {
		return event, nil
	}

	optionalFields := rawEvent[header.titleLength+1+header.textLength+1:]
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
