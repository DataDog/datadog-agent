// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package windowsevent contains utilities to transform Windows Event Log XML messages into structured messages for Datadog Logs.
package windowsevent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/clbanning/mxj"
	"golang.org/x/text/encoding/unicode"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	stringUtil "github.com/DataDog/datadog-agent/pkg/util/strings"
)

const (
	binaryPath  = "Event.EventData.Binary"
	dataPath    = "Event.EventData.Data"
	taskPath    = "Event.System.Task"
	opcodePath  = "Event.System.Opcode"
	eventIDPath = "Event.System.EventID"
	// Custom path, not a Microsoft path
	eventIDQualifierPath = "Event.System.EventIDQualifier"
	maxMessageBytes      = 128 * 1024 // 128 kB
	truncatedFlag        = "...TRUNCATED..."
)

// Map is a wrapper around mxj.Map that provides additional methods to manipulate the map
// as it is used in the context of Windows Event Log messages.
type Map struct {
	mxj.Map
}

// SetTask sets the task field in the map.
func (m *Map) SetTask(task string) error {
	if task == "" {
		return nil
	}
	_, err := m.Map.UpdateValuesForPath("Task:"+task, taskPath)
	return err
}

// SetOpcode sets the opcode field in the map.
func (m *Map) SetOpcode(opcode string) error {
	if opcode == "" {
		return nil
	}
	_, err := m.Map.UpdateValuesForPath("Opcode:"+opcode, opcodePath)
	return err
}

// SetMessage sets the message field in the map. This field is a DD field not a Windows Event Log field.
// The message is truncated if it is bigger than 128kB to prevent it from being dropped.
func (m *Map) SetMessage(message string) error {
	if message == "" {
		return nil
	}
	// Truncates the message. Messages with more than 128kB are likely to be bigger
	// than 256kB when serialized and then dropped
	if len(message) > maxMessageBytes {
		message = stringUtil.TruncateUTF8(message, maxMessageBytes)
		message = message + truncatedFlag
	}
	return m.Map.SetValueForPath(message, "message")
}

// SetLevel sets the level field in the map. This field is a DD field not a Windows Event Log field.
func (m *Map) SetLevel(level string) error {
	if level == "" {
		return nil
	}
	return m.Map.SetValueForPath(level, "level")
}

// JSON returns the map as a JSON byte array.
//
// The function replaces any "#text" key with a "value" key.
func (m *Map) JSON() ([]byte, error) {
	j, err := m.Map.Json(false)
	if err != nil {
		return nil, err
	}
	return replaceTextKeyToValue(j), nil
}

// GetMessage returns the message field from the map.
func (m *Map) GetMessage() string {
	if message, exists := m.Map["message"]; exists {
		return message.(string)
	}
	return ""
}

// NewMapXML converts Windows Event Log XML to a map and runs some transforms to normalize the data.
//
// Transforms:
//   - Event.EventData.Data: Convert to a map if values are named, else to a list
//   - Event.EventData.Binary: Convert to a string if it is a utf-16 string
//   - Event.System.EventID: Separate the EventID and Qualifier fields
func NewMapXML(eventXML []byte) (*Map, error) {
	var err error
	m := &Map{}

	mxj.PrependAttrWithHyphen(false)
	m.Map, err = mxj.NewMapXml(eventXML)
	if err != nil {
		return nil, err
	}

	// extract then modify the Event.EventData.Data field to have a key value mapping
	err = formatEventDataField(m.Map)
	if err != nil {
		log.Debugf("Error formatting %s: %s", dataPath, err)
	}

	// extract, parse then modify the Event.EventData.Binary data field
	err = formatEventBinaryData(m.Map)
	if err != nil {
		log.Debugf("Error formatting %s: %s", binaryPath, err)
	}

	// Normalize the Event.System.EventID field
	err = normalizeEventID(m.Map)
	if err != nil {
		log.Debugf("Error normalizing EventID: %s", err)
	}

	return m, nil
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

// formatEventDataField transforms the fields parsed from <Data Name='NAME1'>VALUE1</Data><Data Name='NAME2'>VALUE2</Data> to
// a map that will be JSON serialized to {"NAME1": "VALUE1", "NAME2": "VALUE2"}
// The Name attribute is optional in the Data schema, so the transform may not apply to all events.
// https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-datafieldtype-complextype
func formatEventDataField(mv mxj.Map) error {
	values, err := mv.ValuesForPath(dataPath)
	if err != nil || len(values) == 0 {
		// Event.EventData.Data is an optional element, so it not existing is not an error
		return nil
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

	if len(nameTextMap) > 0 {
		err = mv.SetValueForPath(nameTextMap, dataPath)
		if err != nil {
			return err
		}
	}
	return nil
}

// formatEventBinaryData formats the field Event.EventData.Binary field.
// The field is optional, so it may not exist in all events.
// If the field exists, its value is a hex string of arbitrary data.
// If the hex string contains a utf-16 string, this function will decode it.
// https://learn.microsoft.com/en-us/windows/win32/wes/eventschema-eventdatatype-complextype
func formatEventBinaryData(mv mxj.Map) error {
	values, err := mv.ValuesForPath(binaryPath)
	if err != nil || len(values) == 0 {
		// Event.EventData.Data is an optional element, so it not existing is not an error
		return nil
	}

	valueString, ok := values[0].(string)
	if !ok {
		return fmt.Errorf("could not cast binary data to string: %w", err)
	}

	// decoded is an utf-16 array of byte
	decodedHex, err := hex.DecodeString(valueString)
	if err != nil {
		return err
	}

	// TODO: compat: binary data is not guaranteed to be a utf-16 string, but go's
	// encode function doesn't return an error, it replaces invalid bytes.
	// But the old log tailer always did this so we're keeping it for compat.
	decodedBytes, err := convertUTF16ToUTF8(decodedHex)
	if err != nil {
		// not an error because Binary field doesn't have to be utf-16 data
		return nil
	}
	// remove null terminator
	str := strings.TrimRight(string(decodedBytes), "\x00")
	_, err = mv.UpdateValuesForPath("Binary:"+str, binaryPath)
	if err != nil {
		return err
	}
	return nil
}

// utf16decode converts ut16le bytes to utf8 bytes
func convertUTF16ToUTF8(b []byte) ([]byte, error) {
	if len(b)%2 != 0 {
		return nil, fmt.Errorf("length must be an even number")
	}
	// UTF-16 little-endian (UTF-16LE) is the encoding standard in the Windows operating system.
	// https://learn.microsoft.com/en-us/globalization/encoding/transformations-of-unicode-code-points
	utf16le := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	decoder := utf16le.NewDecoder()
	return decoder.Bytes(b)
}

// replaceTextKeyValue replaces a "#text" key to a "value" key.
// That happens when a tag has an attribute and a content. E.g. <EventID Qualifiers='16384'>7036</EventID>
func replaceTextKeyToValue(jsonEvent []byte) []byte {
	jsonEvent = bytes.Replace(jsonEvent, []byte("\"#text\":"), []byte("\"value\":"), -1)
	return jsonEvent
}
