// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

// UnmarshalMsg unmarshals a TracerPayload from a byte stream, updating the strings slice with new strings
func (tp *InternalTracerPayload) UnmarshalMsg(bts []byte) (o []byte, err error) {
	if tp.Strings == nil {
		tp.Strings = NewStringTable()
	}
	var numFields uint32
	numFields, o, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read tracer payload fields header")
		return
	}
	for numFields > 0 {
		numFields--
		var fieldNum uint32
		fieldNum, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read tracer payload field")
			return
		}
		switch fieldNum {
		case 1:
			// If strings are sent they must be sent first.
			if tp.Strings.Len() > 1 {
				err = msgp.WrapError(err, "Unexpected strings attribute, strings must be sent first")
				return
			}
			o, err = unmarshalStringTable(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload strings")
				return
			}
		case 2:
			tp.ContainerID, o, err = UnmarshalStreamingString(o, tp.Strings) //TODO: write a test checking if strings can be sent up-front
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload containerID")
				return
			}
		case 3:
			tp.LanguageName, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload languageName")
				return
			}
		case 4:
			tp.LanguageVersion, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload languageVersion")
				return
			}
		case 5:
			tp.TracerVersion, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload tracerVersion")
				return
			}
		case 6:
			tp.RuntimeID, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload runtimeID")
				return
			}
		case 7:
			tp.Env, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload env")
				return
			}
		case 8:
			tp.Hostname, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload hostname")
				return
			}
		case 9:
			tp.AppVersion, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload appVersion")
				return
			}
		case 10:
			tp.Attributes, o, err = UnmarshalKeyValueMap(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload attributes")
				return
			}
		case 11:
			tp.Chunks, o, err = UnmarshalTraceChunkList(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload chunks")
				return
			}
		default:
			fmt.Printf("Unknown tracer payload field number %d\n", fieldNum) //todo: warn log
		}
	}
	return
}

// unmarshalStringTable unmarshals a list of strings from a byte stream
func unmarshalStringTable(bts []byte, strings *StringTable) (o []byte, err error) {
	var numStrings uint32
	numStrings, o, err = msgp.ReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read string list header")
		return
	}
	for numStrings > 0 {
		numStrings--
		var str string
		str, o, err = msgp.ReadStringBytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read string")
		}
		if str == "" {
			continue // Skip empty strings, we already have an empty string at index 0
		}
		strings.addUnchecked(str) //We don't need to check for duplicates because the string table should arrive with only unique strings
	}
	return
}

// UnmarshalTraceChunkList unmarshals a list of TraceChunks from a byte stream, updating the strings slice with new strings
func UnmarshalTraceChunkList(bts []byte, strings *StringTable) (chunks []*InternalTraceChunk, o []byte, err error) {
	var numChunks uint32
	numChunks, o, err = msgp.ReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read trace chunk list header")
		return
	}
	chunks = make([]*InternalTraceChunk, numChunks)
	for i := range chunks {
		chunks[i] = &InternalTraceChunk{Strings: strings}
		o, err = chunks[i].UnmarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, fmt.Sprintf("Failed to read trace chunk %d", i))
			return
		}
	}
	return
}

// UnmarshalMsg unmarshals a TraceChunk from a byte stream, updating the strings slice with new strings
func (tc *InternalTraceChunk) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var numFields uint32
	numFields, o, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "Failed to read trace chunk fields header")
		return
	}
	for numFields > 0 {
		numFields--
		var fieldNum uint32
		fieldNum, o, err = msgp.ReadUint32Bytes(o)
		if err != nil {
			err = msgp.WrapError(err, "Failed to read trace chunk field")
			return
		}
		switch fieldNum {
		case 1:
			tc.Priority, o, err = msgp.ReadInt32Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk priority")
				return
			}
		case 2:
			tc.Origin, o, err = UnmarshalStreamingString(o, tc.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk origin")
				return
			}
		case 3:
			tc.Attributes, o, err = UnmarshalKeyValueMap(o, tc.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk attributes")
				return
			}
		case 4:
			tc.Spans, o, err = UnmarshalSpanList(o, tc.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk spans")
				return
			}
		case 5:
			tc.DroppedTrace, o, err = msgp.ReadBoolBytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk droppedTrace")
				return
			}
		case 6:
			tc.TraceID, o, err = msgp.ReadBytesBytes(o, nil)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk traceID")
				return
			}
		case 7:
			tc.DecisionMaker, o, err = UnmarshalStreamingString(o, tc.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk decisionMaker")
				return
			}
		default:
			fmt.Printf("Unknown trace chunk field number %d\n", fieldNum)
			return
		}
	}
	return
}
