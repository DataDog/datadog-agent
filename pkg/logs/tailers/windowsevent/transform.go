// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package windowsevent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/clbanning/mxj"
	"golang.org/x/text/encoding/unicode"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eventToJSON converts an XML message into either an unstructured message.Message
// or a structured one.
func eventToMessage(re *richEvent, source *sources.LogSource, processRawMessage bool) (*message.Message, error) {
	event := re.xmlEvent
	log.Trace("Rendered XML:", event)
	mxj.PrependAttrWithHyphen(false)
	mv, err := mxj.NewMapXml([]byte(event))
	if err != nil {
		return nil, err
	}

	// extract then modify the Event.EventData.Data field to have a key value mapping
	err = formatEventDataField(mv)
	if err != nil {
		log.Debugf("Error formatting %s: %s", dataPath, err)
	}

	// extract, parse then modify the Event.EventData.Binary data field
	err = formatEventBinaryData(mv)
	if err != nil {
		log.Debugf("Error formatting %s: %s", binaryPath, err)
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

	// old behaviour using an unstructured message with raw data
	if processRawMessage {
		jsonEvent, err := mv.Json(false)
		if err != nil {
			return nil, err
		}
		jsonEvent = replaceTextKeyToValue(jsonEvent)
		log.Trace("Transformed JSON:", string(jsonEvent))
		return message.NewMessageWithSource(jsonEvent, message.StatusInfo, source, time.Now().UnixNano()), nil
	}

	// new behaviour returning a structured message
	return message.NewStructuredMessage(
		&WindowsEventMessage{data: mv},
		message.NewOrigin(source),
		message.StatusInfo,
		time.Now().UnixNano(),
	), nil
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
