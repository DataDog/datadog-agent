// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsevent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/clbanning/mxj"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	binaryPath  = "Event.EventData.Binary"
	dataPath    = "Event.EventData.Data"
	taskPath    = "Event.System.Task"
	opcode      = "Event.System.Opcode"
	eventIDPath = "Event.System.EventID"
	// Custom path, not a Microsoft path
	eventIDQualifierPath = "Event.System.EventIDQualifier"
)

// Config is a event log tailer configuration
type Config struct {
	ChannelPath string
	Query       string
}

// eventContext links go and c
type eventContext struct {
	id int
}

// richEvent carries rendered information to create a richer log
type richEvent struct {
	xmlEvent string
	message  string
	task     string
	opcode   string
	level    string
}

// Tailer collects logs from event log.
type Tailer struct {
	source     *sources.LogSource
	config     *Config
	outputChan chan *message.Message
	stop       chan struct{}
	done       chan struct{}

	context *eventContext
}

// NewTailer returns a new tailer.
func NewTailer(source *sources.LogSource, config *Config, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		source:     source,
		config:     config,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Identifier returns a string that uniquely identifies a source
func Identifier(channelPath, query string) string {
	return fmt.Sprintf("eventlog:%s;%s", channelPath, query)
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return Identifier(t.config.ChannelPath, t.config.Query)
}

// toMessage converts an XML message into json
func (t *Tailer) toMessage(re *richEvent) (*message.Message, error) { //nolint:unused
	event := re.xmlEvent
	log.Debug("Rendered XML:", event)
	mxj.PrependAttrWithHyphen(false)
	mv, err := mxj.NewMapXml([]byte(event))
	if err != nil {
		return &message.Message{}, err
	}

	// extract then modify the Event.EventData.Data field to have a key value mapping
	dataField, err := extractDataField(mv)
	if err != nil {
		log.Debugf("Error extracting data field: %s", err)
	} else {
		err = mv.SetValueForPath(dataField, dataPath)
		if err != nil {
			log.Debugf("Error formatting %s: %s", dataPath, err)
		}
	}

	// extract, parse then modify the Event.EventData.Binary data field
	binaryData, err := extractParsedBinaryData(mv)
	if err != nil {
		log.Debugf("Error extracting binary data: %s", err)
	} else {
		_, err = mv.UpdateValuesForPath("Binary:"+binaryData, binaryPath)
		if err != nil {
			log.Debugf("Error formatting %s: %s", binaryPath, err)
		}
	}

	// Normalize the Event.System.EventID field
	err = normalizeEventID(mv)
	if err != nil {
		log.Debugf("Error normalizing EventID: %s", err)
	}

	// Replace Task and Opcode codes by the rendered value
	if re.task != "" {
		_, _ = mv.UpdateValuesForPath("Task:"+re.task, taskPath)
	}
	if re.opcode != "" {
		_, _ = mv.UpdateValuesForPath("Opcode:"+re.opcode, opcode)
	}
	// Set message and severity
	if re.message != "" {
		_ = mv.SetValueForPath(re.message, "message")
	}
	if re.level != "" {
		_ = mv.SetValueForPath(re.level, "level")
	}

	jsonEvent, err := mv.Json(false)
	if err != nil {
		return &message.Message{}, err
	}
	jsonEvent = replaceTextKeyToValue(jsonEvent)
	log.Debug("Sending JSON:", string(jsonEvent))
	return message.NewMessageWithSource(jsonEvent, message.StatusInfo, t.source, time.Now().UnixNano()), nil
}

// EventID sometimes comes in like <EventID>7036</EventID>
//
//	which mxj will transform to "EventID":"7036"
//
// other times it comes in like <EventID Qualifiers='16384'>7036</EventID>
//
//	which mxj will transform to "EventID":{"value":"7036","Qualifiers":"16384"}
//
// We want to normalize this so the resulting JSON is consistent
//
//	"EventID":"7036","EventIDQualifier":"16384"
//
// Format definition: https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-systempropertiestype-complextype
func normalizeEventID(mv mxj.Map) error {
	values, err := mv.ValuesForPath(eventIDPath)
	if err != nil || len(values) == 0 {
		return fmt.Errorf("could not find path: %s", eventIDPath)
	}
	for _, value := range values {
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		// Get element value
		text, foundText := valueMap["#text"]
		// Qualifier optional
		qualifier, foundQualifier := valueMap["Qualifiers"]
		if foundText && foundQualifier {
			// Remove Qualifiers attribute from EventID by
			// overwriting the path with just the text value
			_ = mv.SetValueForPath(text, eventIDPath)
			// Add qualifier value to a new path
			_ = mv.SetValueForPath(qualifier, eventIDQualifierPath)
		}
	}
	return nil
}

// extractDataField transforms the fields parsed from <Data Name='NAME1'>VALUE1</Data><Data Name='NAME2'>VALUE2</Data> to
// a map that will be JSON serialized to {"NAME1": "VALUE1", "NAME2": "VALUE2"}
// Data fields always have this schema:
// https://docs.microsoft.com/en-us/windows/desktop/WES/eventschema-complexdatatype-complextype
func extractDataField(mv mxj.Map) (map[string]interface{}, error) {
	values, err := mv.ValuesForPath(dataPath)
	if err != nil || len(values) == 0 {
		return nil, fmt.Errorf("could not find path: %s", dataPath)
	}
	nameTextMap := make(map[string]interface{})
	for _, value := range values {
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		name, foundName := valueMap["Name"]
		text, foundText := valueMap["#text"]
		if !foundName || !foundText {
			continue
		}
		nameString, ok := name.(string)
		if !ok {
			continue
		}
		nameTextMap[nameString] = text
	}
	if len(nameTextMap) == 0 {
		return nil, fmt.Errorf("no field to transform")
	}
	return nameTextMap, nil
}

// extractParsedBinaryData extract the field Event.EventData.Binary and parse it to its string value
func extractParsedBinaryData(mv mxj.Map) (string, error) {
	values, err := mv.ValuesForPath(binaryPath)
	if err != nil || len(values) == 0 {
		return "", fmt.Errorf("could not find path: %s", binaryPath)
	}
	valueString, ok := values[0].(string)
	if !ok {
		return "", fmt.Errorf("could not cast binary data to string: %s", err)
	}

	decodedString, _ := parseBinaryData(valueString)
	if err != nil {
		return "", fmt.Errorf("could not decode %s: %s", valueString, err)
	}

	return decodedString, nil
}

// parseBinaryData parses the hex string found in the field Event.EventData.Binary to an UTF-8 valid string
func parseBinaryData(s string) (string, error) {
	// decoded is an utf-16 array of byte
	decodedHex, err := decodeHex(s)
	if err != nil {
		return "", err
	}

	utf8String, err := decodeUTF16(decodedHex)
	if err != nil {
		return "", err
	}

	// The string might be utf16 null-terminated (2 null bytes)
	parsedString := strings.TrimRight(utf8String, "\x00")
	return parsedString, nil
}

// decodeHex reads an hexadecimal string to an array of bytes
func decodeHex(s string) ([]byte, error) {
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return []byte(nil), err
	}

	return decoded, nil
}

// decodeUTF16 transforms an array of bytes of an UTF-16 string to an UTF-8 string
func decodeUTF16(b []byte) (string, error) {
	// https://gist.github.com/bradleypeabody/185b1d7ed6c0c2ab6cec
	if len(b)%2 != 0 {
		return "", fmt.Errorf("Must have even length byte slice")
	}
	ret := &bytes.Buffer{}
	u16s := make([]uint16, 1)
	b8buf := make([]byte, 4)

	lb := len(b)
	for i := 0; i < lb; i += 2 {
		u16s[0] = uint16(b[i]) + (uint16(b[i+1]) << 8)
		r := utf16.Decode(u16s)
		n := utf8.EncodeRune(b8buf, r[0])
		ret.Write(b8buf[:n])
	}

	return ret.String(), nil
}

// replaceTextKeyValue replaces a "#text" key to a "value" key.
// That happens when a tag has an attribute and a content. E.g. <EventID Qualifiers='16384'>7036</EventID>
func replaceTextKeyToValue(jsonEvent []byte) []byte {
	jsonEvent = bytes.Replace(jsonEvent, []byte("\"#text\":"), []byte("\"value\":"), -1)
	return jsonEvent
}
