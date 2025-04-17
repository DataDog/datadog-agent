// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package eventparser is used for parsing raw bytes from bpf code into events
package eventparser

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ratelimiter"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/unix"
)

var (
	byteOrder = binary.NativeEndian
)

// ParseEvent takes the raw buffer from bpf and parses it into an event. It also potentially
// applies a rate limit
func ParseEvent(record []byte, ratelimiters *ratelimiter.MultiProbeRateLimiter) (*ditypes.DIEvent, error) {
	event := ditypes.DIEvent{}

	if len(record) < ditypes.SizeofBaseEvent {
		return nil, fmt.Errorf("malformed event record (length %d)", len(record))
	}
	baseEvent := *(*ditypes.BaseEvent)(unsafe.Pointer(&record[0]))
	event.ProbeID = unix.ByteSliceToString(baseEvent.Probe_id[:])

	allowed, droppedEvents, successfulEvents := ratelimiters.AllowOneEvent(event.ProbeID)
	if !allowed {
		return nil, fmt.Errorf("event dropped by rate limit, probe %s (%d dropped events out of %d)",
			event.ProbeID, droppedEvents, droppedEvents+successfulEvents)
	}

	event.PID = baseEvent.Pid
	event.UID = baseEvent.Uid
	event.StackPCs = baseEvent.Program_counters[:]
	event.ParamIndicies = baseEvent.Param_indicies[:]

	event.Argdata = readParams(event.ParamIndicies, record[ditypes.SizeofBaseEvent:])
	return &event, nil
}

func readParams(indicies []uint64, values []byte) []*ditypes.Param {
	if len(values) >= 100 {
		log.Tracef("DI event bytes (0:100): %v", values[0:100])
	}
	outputParams := []*ditypes.Param{}
	for i := range indicies {
		if i != 0 && indicies[i] == 0 {
			break
		}
		if uint64(len(values)) <= indicies[i] {
			break
		}
		data := values[indicies[i]:]
		paramTypeDefinition := parseTypeDefinition(data)
		if paramTypeDefinition == nil {
			break
		}
		sizeOfTypeDefinition := countBufferUsedByTypeDefinition(paramTypeDefinition)
		val, _ := parseParamValue(paramTypeDefinition, data[sizeOfTypeDefinition:])
		outputParams = append(outputParams, val)
	}
	return outputParams
}

// parseParamValue takes the representation of the param type's definition and the
// actual values in the buffer and populates the definition with the value parsed
// from the byte buffer. It returns the resulting parameter and an indication of
// how many bytes were read from the buffer
func parseParamValue(definition *ditypes.Param, buffer []byte) (*ditypes.Param, int) {
	if definition == nil {
		return nil, 0
	}
	if definition.Size == 0 {
		// The definitions size can be zero in cases like empty slices or
		// structs with no fields
		definition.Fields = nil
		return definition, 0
	}

	// Start by creating a stack with each layer of the definition
	// which will correspond with the layers of the values read from buffer.
	// This is done using a temporary stack to reverse the order.
	var bufferIndex int
	tempStack := newParamStack()
	definitionStack := newParamStack()
	tempStack.push(definition)
	for !tempStack.isEmpty() {
		current := tempStack.pop()
		copiedParam := copyParam(current)
		if current.Kind == byte(reflect.Pointer) {
			// Pointers have special logic because they have their own
			// values (address) in addition to the value they point at.
			// The pointer is pushed after the value, unlike other types
			// that have sub-types like slices or structs.
			if len(current.Fields) != 1 {
				return definition, 0
			}
			definitionStack.push(current.Fields[0])
			definitionStack.push(copiedParam)
			continue
		}
		definitionStack.push(copiedParam)
		if current.Size == 0 {
			continue
		}
		for n := 0; n < len(current.Fields); n++ {
			tempStack.push(current.Fields[n])
		}
	}

	valueStack := newParamStack()
	// Iterate over buffer and parameter definition stack to parse values
	// into corresponding types.
	for bufferIndex <= len(buffer) {
		paramDefinition := definitionStack.pop()
		if paramDefinition == nil {
			break
		}
		nextIndex := bufferIndex + int(paramDefinition.Size)

		if reflect.Kind(paramDefinition.Kind) == reflect.String {
			if nextIndex > len(buffer) {
				break
			}
			paramDefinition.ValueStr = string(buffer[bufferIndex:nextIndex])
			bufferIndex += int(paramDefinition.Size)
			valueStack.push(paramDefinition)

		} else if !isTypeWithHeader(paramDefinition.Kind) {
			if nextIndex > len(buffer) {
				break
			}

			// This is a regular value (no sub-fields).
			// We parse the value of it from the buffer and push it to the value stack
			paramDefinition.ValueStr = parseIndividualValue(paramDefinition.Kind, buffer[bufferIndex:nextIndex])
			bufferIndex += int(paramDefinition.Size)
			valueStack.push(paramDefinition)

		} else if reflect.Kind(paramDefinition.Kind) == reflect.Pointer {
			if nextIndex > len(buffer) {
				break
			}
			paramDefinition.ValueStr = parseIndividualValue(paramDefinition.Kind, buffer[bufferIndex:nextIndex])
			bufferIndex += int(paramDefinition.Size)
			pointerActualValueDefinition := definitionStack.pop()
			pointerActualValue, ind := parseParamValue(pointerActualValueDefinition, buffer[bufferIndex:])
			bufferIndex += ind
			if paramDefinition.ValueStr != "0x0" {
				paramDefinition.Fields = append(paramDefinition.Fields, pointerActualValue)
			}
			valueStack.push(paramDefinition)

		} else {
			// This is a type with sub-fields which have already been parsed and push
			// onto the value stack. We pop those and set them as fields in this type.
			// We then push this type onto the value stack as it may also be a sub-field.
			// In header types like this, paramDefinition.Size corresponds with the number of
			// fields under it.
			for n := 0; n < int(paramDefinition.Size); n++ {
				paramDefinition.Fields = append([]*ditypes.Param{valueStack.pop()}, paramDefinition.Fields...)
			}
			valueStack.push(paramDefinition)
		}
	}

	return valueStack.pop(), bufferIndex
}

func deepCopyParam(dst, src *ditypes.Param) {
	dst.Type = src.Type
	dst.Kind = src.Kind
	dst.Size = src.Size
	dst.Fields = make([]*ditypes.Param, len(src.Fields))
	for i, field := range src.Fields {
		dst.Fields[i] = &ditypes.Param{}
		deepCopyParam(dst.Fields[i], field)
	}
}

func copyParam(p *ditypes.Param) *ditypes.Param {
	return &ditypes.Param{
		Type: p.Type,
		Kind: p.Kind,
		Size: p.Size,
	}
}

func parseKindToString(kind byte) string {
	if kind == 255 {
		return "Unsupported"
	} else if kind == 254 {
		return "reached field limit"
	}
	return reflect.Kind(kind).String()
}

// parseTypeDefinition is given a buffer which contains the header type definition
// for basic/complex types, and the actual content of those types.
// It returns a fully populated tree of `ditypes.Param` which will be used for parsing
// the actual values
func parseTypeDefinition(b []byte) *ditypes.Param {
	stack := newParamStack()
	i := 0
	for {
		if len(b) < i+sizeOfKindAndSize {
			log.Tracef("could not parse type definition, ran out of buffer while parsing")
			return nil
		}
		kind := b[i]
		newParam := &ditypes.Param{
			Kind: kind,
			Size: byteOrder.Uint16(b[i+1 : i+sizeOfKindAndSize]),
			Type: parseKindToString(kind),
		}
		if newParam.Kind == 0 {
			goto stackCheck
		}
		i += sizeOfKindAndSize
		if newParam.Size == 0 {
			if reflect.Kind(newParam.Kind) == reflect.Struct {
				goto stackCheck
			}
		}
		if isTypeWithHeader(newParam.Kind) {
			stack.push(newParam)
			continue
		}

	stackCheck:
		if stack.isEmpty() {
			return newParam
		}
		top := stack.peek()
		top.Fields = append(top.Fields, newParam)

		if reflect.Kind(top.Kind) == reflect.Slice {
			// top.Size is the length of the slice.
			// We copy+append the type of the slice so we have the correct
			// number of slice elements to parse values into.
			// There's a special implicit logic for top.Size == 0 in which
			// case we want the field for the underlying type just for
			// displaying context to user, but we're not expecting to
			// populate it with parsed values.
			if len(top.Fields) > 0 {
				top.Type = fmt.Sprintf("[]%s", top.Fields[0].Type)
			}
			for q := 1; q < int(top.Size); q++ {
				sliceElementTypeCopy := &ditypes.Param{}
				deepCopyParam(sliceElementTypeCopy, top.Fields[0])
				top.Fields = append(top.Fields, sliceElementTypeCopy)
			}
		}

		if reflect.Kind(top.Kind) == reflect.Pointer &&
			len(top.Fields) > 0 {
			top.Type = fmt.Sprintf("*%s", top.Fields[0].Type)
		}

		if len(top.Fields) == int(top.Size) ||
			(reflect.Kind(top.Kind) == reflect.Slice && top.Size == 0) ||
			(reflect.Kind(top.Kind) == reflect.Pointer && len(top.Fields) == 1) {
			newParam = stack.pop()
			goto stackCheck
		}
	}
}

// countBufferUsedByTypeDefinition is used to determine that amount of bytes
// that were used to read the type definition. Each individual element of the
// definition uses 3 bytes (1 for kind, 2 for size). This is a needed calculation
// so we know where we should read the actual values in the buffer.
func countBufferUsedByTypeDefinition(root *ditypes.Param) int {
	queue := []*ditypes.Param{root}
	counter := 0
	for len(queue) != 0 {
		front := queue[0]
		queue = queue[1:]
		counter += sizeOfKindAndSize

		if reflect.Kind(front.Kind) == reflect.Slice && len(front.Fields) > 0 {
			// The fields of slice elements are amended after the fact to account
			// for the runtime discovered length. However, only one definition of
			// the slice element's type is present in the buffer.
			queue = append(queue, front.Fields[0])
		} else {
			queue = append(queue, front.Fields...)
		}
	}
	return counter
}

func isTypeWithHeader(pieceType byte) bool {
	return reflect.Kind(pieceType) == reflect.Struct ||
		reflect.Kind(pieceType) == reflect.Array ||
		reflect.Kind(pieceType) == reflect.Slice ||
		reflect.Kind(pieceType) == reflect.Pointer
}

func parseIndividualValue(paramType byte, paramValueBytes []byte) string {
	switch reflect.Kind(paramType) {
	case reflect.Uint8:
		if len(paramValueBytes) < 1 {
			return "insufficient data for uint8"
		}
		return fmt.Sprintf("%d", uint8(paramValueBytes[0]))
	case reflect.Int8:
		if len(paramValueBytes) < 1 {
			return "insufficient data for int8"
		}
		return fmt.Sprintf("%d", int8(paramValueBytes[0]))
	case reflect.Uint16:
		if len(paramValueBytes) < 2 {
			return "insufficient data for uint16"
		}
		return fmt.Sprintf("%d", byteOrder.Uint16(paramValueBytes))
	case reflect.Int16:
		if len(paramValueBytes) < 2 {
			return "insufficient data for int16"
		}
		return fmt.Sprintf("%d", int16(byteOrder.Uint16(paramValueBytes)))
	case reflect.Uint32:
		if len(paramValueBytes) < 4 {
			return "insufficient data for uint32"
		}
		return fmt.Sprintf("%d", byteOrder.Uint32(paramValueBytes))
	case reflect.Int32:
		if len(paramValueBytes) < 4 {
			return "insufficient data for int32"
		}
		return fmt.Sprintf("%d", int32(byteOrder.Uint32(paramValueBytes)))
	case reflect.Uint64:
		if len(paramValueBytes) < 8 {
			return "insufficient data for uint64"
		}
		return fmt.Sprintf("%d", byteOrder.Uint64(paramValueBytes))
	case reflect.Int64:
		if len(paramValueBytes) < 8 {
			return "insufficient data for int64"
		}
		return fmt.Sprintf("%d", int64(byteOrder.Uint64(paramValueBytes)))
	case reflect.Uint:
		if len(paramValueBytes) < 8 {
			return "insufficient data for uint"
		}
		return fmt.Sprintf("%d", byteOrder.Uint64(paramValueBytes))
	case reflect.Int:
		if len(paramValueBytes) < 8 {
			return "insufficient data for int"
		}
		return fmt.Sprintf("%d", int(byteOrder.Uint64(paramValueBytes)))
	case reflect.Pointer:
		if len(paramValueBytes) < 8 {
			return "insufficient data for pointer"
		}
		return fmt.Sprintf("0x%X", byteOrder.Uint64(paramValueBytes))
	case reflect.String:
		return string(paramValueBytes)
	case reflect.Bool:
		if len(paramValueBytes) == 0 || paramValueBytes[0] == 0 {
			return "false"
		}
		return "true"
	case ditypes.KindUnsupported:
		return "UNSUPPORTED"
	default:
		return ""
	}
}

const sizeOfKindAndSize = 3
