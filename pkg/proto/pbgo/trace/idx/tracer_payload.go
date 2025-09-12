// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

const maxSize = 25 * 1e6 // maxSize protects the decoder from payloads lying about their size

// UnmarshalMsg unmarshals a TracerPayload from a byte stream, updating the strings slice with new strings
// Returns any leftover bytes after the tracer payload is unmarshalled and any error that occurred
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
			// TODO: this should always be an error
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
			tp.containerIDRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload containerID")
				return
			}
		case 3:
			tp.languageNameRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload languageName")
				return
			}
		case 4:
			tp.languageVersionRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload languageVersion")
				return
			}
		case 5:
			tp.tracerVersionRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload tracerVersion")
				return
			}
		case 6:
			tp.runtimeIDRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload runtimeID")
				return
			}
		case 7:
			tp.envRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload env")
				return
			}
		case 8:
			tp.hostnameRef, o, err = UnmarshalStreamingString(o, tp.Strings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read tracer payload hostname")
				return
			}
		case 9:
			tp.appVersionRef, o, err = UnmarshalStreamingString(o, tp.Strings)
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

// MarshalMsg marshals a TracerPayload into a byte stream
func (tp *InternalTracerPayload) MarshalMsg(bts []byte) (o []byte, err error) {
	serStrings := NewSerializedStrings(uint32(tp.Strings.Len()))
	// Count non-default fields to determine map header size
	numFields := 0
	if tp.containerIDRef != 0 {
		numFields++
	}
	if tp.languageNameRef != 0 {
		numFields++
	}
	if tp.languageVersionRef != 0 {
		numFields++
	}
	if tp.tracerVersionRef != 0 {
		numFields++
	}
	if tp.runtimeIDRef != 0 {
		numFields++
	}
	if tp.envRef != 0 {
		numFields++
	}
	if tp.hostnameRef != 0 {
		numFields++
	}
	if tp.appVersionRef != 0 {
		numFields++
	}
	if len(tp.Attributes) > 0 {
		numFields++
	}
	if len(tp.Chunks) > 0 {
		numFields++
	}
	o = msgp.AppendMapHeader(bts, uint32(numFields))
	if tp.containerIDRef != 0 {
		o = msgp.AppendUint32(o, 2) // containerID
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.containerIDRef), tp.containerIDRef, o)
	}
	if tp.languageNameRef != 0 {
		o = msgp.AppendUint32(o, 3) // languageName
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.languageNameRef), tp.languageNameRef, o)
	}
	if tp.languageVersionRef != 0 {
		o = msgp.AppendUint32(o, 4) // languageVersion
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.languageVersionRef), tp.languageVersionRef, o)
	}
	if tp.tracerVersionRef != 0 {
		o = msgp.AppendUint32(o, 5) // tracerVersion
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.tracerVersionRef), tp.tracerVersionRef, o)
	}
	if tp.runtimeIDRef != 0 {
		o = msgp.AppendUint32(o, 6) // runtimeID
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.runtimeIDRef), tp.runtimeIDRef, o)
	}
	if tp.envRef != 0 {
		o = msgp.AppendUint32(o, 7) // env
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.envRef), tp.envRef, o)
	}
	if tp.hostnameRef != 0 {
		o = msgp.AppendUint32(o, 8) // hostname
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.hostnameRef), tp.hostnameRef, o)
	}
	if tp.appVersionRef != 0 {
		o = msgp.AppendUint32(o, 9) // appVersion
		o = serStrings.AppendStreamingString(tp.Strings.Get(tp.appVersionRef), tp.appVersionRef, o)
	}
	if len(tp.Attributes) > 0 {
		o = msgp.AppendUint32(o, 10) // attributes
		o, err = MarshalAttributesMap(o, tp.Attributes, tp.Strings, serStrings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to marshal attributes")
			return
		}
	}
	if len(tp.Chunks) > 0 {
		o = msgp.AppendUint32(o, 11) // chunks
		o = msgp.AppendArrayHeader(o, uint32(len(tp.Chunks)))
		for _, chunk := range tp.Chunks {
			o, err = chunk.MarshalMsg(o, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal trace chunk")
				return
			}
		}
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
			tc.originRef, o, err = UnmarshalStreamingString(o, tc.Strings)
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
			tc.samplingMechanism, o, err = msgp.ReadUint32Bytes(o)
			if err != nil {
				err = msgp.WrapError(err, "Failed to read trace chunk samplingMechanism")
				return
			}
		default:
			fmt.Printf("Unknown trace chunk field number %d\n", fieldNum)
			return
		}
	}
	return
}

// MarshalMsg marshals a TraceChunk into a byte stream
func (tc *InternalTraceChunk) MarshalMsg(bts []byte, serStrings *SerializedStrings) (o []byte, err error) {
	// Count non-default fields to determine map header size
	numFields := 0
	if tc.Priority != 0 {
		numFields++
	}
	if tc.originRef != 0 {
		numFields++
	}
	if len(tc.Attributes) > 0 {
		numFields++
	}
	if len(tc.Spans) > 0 {
		numFields++
	}
	if tc.DroppedTrace {
		numFields++
	}
	if len(tc.TraceID) > 0 {
		numFields++
	}
	if tc.samplingMechanism != 0 {
		numFields++
	}
	o = msgp.AppendMapHeader(bts, uint32(numFields))
	if tc.Priority != 0 {
		o = msgp.AppendUint32(o, 1) // priority
		o = msgp.AppendInt32(o, tc.Priority)
	}
	if tc.originRef != 0 {
		o = msgp.AppendUint32(o, 2) // origin
		o = serStrings.AppendStreamingString(tc.Strings.Get(tc.originRef), tc.originRef, o)
	}
	if len(tc.Attributes) > 0 {
		o = msgp.AppendUint32(o, 3) // attributes
		o, err = MarshalAttributesMap(o, tc.Attributes, tc.Strings, serStrings)
		if err != nil {
			err = msgp.WrapError(err, "Failed to marshal attributes")
			return
		}
	}
	if len(tc.Spans) > 0 {
		o = msgp.AppendUint32(o, 4) // spans
		o = msgp.AppendArrayHeader(o, uint32(len(tc.Spans)))
		for _, span := range tc.Spans {
			o, err = span.MarshalMsg(o, serStrings)
			if err != nil {
				err = msgp.WrapError(err, "Failed to marshal span")
				return
			}
		}
	}
	if tc.DroppedTrace {
		o = msgp.AppendUint32(o, 5) // droppedTrace
		o = msgp.AppendBool(o, tc.DroppedTrace)
	}
	if len(tc.TraceID) > 0 {
		o = msgp.AppendUint32(o, 6) // traceID
		o = msgp.AppendBytes(o, tc.TraceID)
	}
	if tc.samplingMechanism != 0 {
		o = msgp.AppendUint32(o, 7) // samplingMechanism
		o = msgp.AppendUint32(o, tc.samplingMechanism)
	}
	return
}
