// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/tinylib/msgp/msgp"
)

const maxSize = 25 * 1e6 // maxSize protects the decoder from payloads lying about their size

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
			tp.ContainerIDRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload containerID")
				return
			}
		case 3:
			tp.LanguageNameRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload languageName")
				return
			}
		case 4:
			tp.LanguageVersionRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload languageVersion")
				return
			}
		case 5:
			tp.TracerVersionRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload tracerVersion")
				return
			}
		case 6:
			tp.RuntimeIDRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload runtimeID")
				return
			}
		case 7:
			tp.EnvRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload env")
				return
			}
		case 8:
			tp.HostnameRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload hostname")
				return
			}
		case 9:
			tp.AppVersionRef, o, err = UnmarshalStreamingString(o, tp.Strings)
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
			log.Warnf("Unknown tracer payload field number %d, are you running the latest agent version?\n", fieldNum)
		}
	}
	return
}

func limitedReadArrayHeaderBytes(bts []byte) (sz uint32, o []byte, err error) {
	sz, o, err = msgp.ReadArrayHeaderBytes(bts)
	if err != nil {
		return
	}
	if sz > maxSize {
		err = msgp.WrapError(err, "Array too large")
		return
	}
	return
}

func limitedReadMapHeaderBytes(bts []byte) (sz uint32, o []byte, err error) {
	sz, o, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		return
	}
	if sz > maxSize {
		err = msgp.WrapError(err, "Map too large")
		return
	}
	return
}

// unmarshalStringTable unmarshals a list of strings from a byte stream
func unmarshalStringTable(bts []byte, strings *StringTable) (o []byte, err error) {
	var numStrings uint32
	numStrings, o, err = limitedReadArrayHeaderBytes(bts)
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
	numChunks, o, err = limitedReadArrayHeaderBytes(bts)
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
			tc.OriginRef, o, err = UnmarshalStreamingString(o, tc.Strings)
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
			tc.DecisionMakerRef, o, err = UnmarshalStreamingString(o, tc.Strings)
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
