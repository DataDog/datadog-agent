package eventparser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/di/ratelimiter"
)

var (
	byteOrder = binary.LittleEndian
)

const MAX_BUFFER_SIZE = 10000 // TODO: find this out from configuration, maybe a special value at the begining of all events?

func ParseEvent(record []byte, ratelimiters *ratelimiter.MultiProbeRateLimiter) *ditypes.DIEvent {
	event := ditypes.DIEvent{}

	if len(record) < 392 {
		log.Println("malformed event record")
		return nil
	}

	indexOfFirstNull := bytes.Index(record[0:304], []byte{0})
	event.ProbeID = string(record[0:indexOfFirstNull])

	allowed, _, _ := ratelimiters.AllowOneEvent(event.ProbeID)
	if !allowed {
		// log.Printf("event dropped by rate limit. Probe %s\t(%d dropped events out of %d)\n",
		// 	event.ProbeID, droppedEvents, droppedEvents+successfulEvents)
		return nil
	}

	event.PID = byteOrder.Uint32(record[304:308])
	event.UID = byteOrder.Uint32(record[308:312])
	event.StackPCs = record[312:392]
	event.Argdata = readParams(record[392:])

	return &event
}

// ParseParms extracts just the parsed parameters from the full event record
func ParseParams(record []byte) ([]ditypes.Param, error) {
	if len(record) < 392 {
		log.Println("malformed event record")
		return nil, fmt.Errorf("malformed event record (length %d)", len(record))
	}
	return readParams(record[392:]), nil
}

func readParams(values []byte) []ditypes.Param {
	outputParams := []ditypes.Param{}
	stack := newParamStack()

	for i := 0; i < MAX_BUFFER_SIZE; {
		if i+3 >= len(values) {
			break
		}

		pieceType := values[i]
		pieceSize := binary.LittleEndian.Uint16(values[i+1 : i+3])

		if pieceType == 0 {
			break
		}

		newParam := &ditypes.Param{
			Size: pieceSize,
			Kind: reflect.Kind(pieceType).String(),
		}

		if isTypeWithHeader(pieceType) {
			// It's a type with a header, pushing onto the stack
			stack.push(newParam)

			if reflect.Kind(pieceType) == reflect.Pointer {
				// Pointers are a unique case where they have fields below them (header)
				// but also have a value (the actual address)
				newParam.Size = 2
			} else {
				i += 3
				continue
			}
		}

		// Not a type with a header, parsing the value
		start, end := i+3, i+3+int(pieceSize)
		if end >= len(values) {
			log.Println("size of parameter piece exceeds length of buffer, dropping data")
			break
		}

		paramValueBytes := values[start:end]
		val := parseParamValue(pieceType, paramValueBytes)
		newParam.ValueStr = val
		i = end

	stackCheck:
		if !stack.isEmpty() {
			// There's an element on the stack, add this parameter to its fields
			// and then check if it should be popped from the stack
			top := stack.peek()

			if !(top.Kind == "ptr" && newParam.Kind == "ptr") {
				// The value of pointers are in the header, we don't want to add another layer
				// to the event with duplicate information
				top.Fields = append(top.Fields, *newParam)
			}

			// If this is a statically sized type (arrays, structs), the size reflects
			// the number of elements instead of total size. Pointers are also included
			// because they always have two values, the address and actual content.
			if top.Kind == reflect.Struct.String() || top.Kind == reflect.Array.String() ||
				top.Kind == reflect.Pointer.String() {
				top.Size -= 1
			}

			// If this is a dynamically sized type (slices), the size reflects
			// the total size of all elements
			if top.Kind == reflect.Slice.String() {
				top.Size -= newParam.Size
			}

			// Check if all fields for the element at the top of the stack
			// have been read
			if top.Size == 0 {
				stack.pop()

				// After popping this element, check if theres more left on the stack,
				// meaning that what was just popped is something like an embedded struct
				if !stack.isEmpty() {
					val := *top
					newParam = &val
					// After popping the stack, there's more left on the stack, so what's popped should be a member of that
					goto stackCheck
				}

				// Otherwise it should just be part of output, meaning
				// it's a top level argument
				outputParams = append(outputParams, *top)

			}
		} else {
			// There's nothing on the stack, just putting the param into output
			outputParams = append(outputParams, *newParam)
		}

	}

	return outputParams
}

func parseParamValue(paramType byte, paramValueBytes []byte) string {

	switch reflect.Kind(paramType) {
	case reflect.Uint8:
		return fmt.Sprintf("%d", uint8(byteOrder.Uint16(paramValueBytes[0:8])))
	case reflect.Int8:
		return fmt.Sprintf("%d", int8(byteOrder.Uint16(paramValueBytes[0:8])))
	case reflect.Uint16:
		return fmt.Sprintf("%d", byteOrder.Uint16(paramValueBytes))
	case reflect.Int16:
		return fmt.Sprintf("%d", int16(byteOrder.Uint16(paramValueBytes)))
	case reflect.Uint32:
		return fmt.Sprintf("%d", byteOrder.Uint32(paramValueBytes))
	case reflect.Int32:
		return fmt.Sprintf("%d", int32(byteOrder.Uint32(paramValueBytes)))
	case reflect.Uint64:
		return fmt.Sprintf("%d", byteOrder.Uint64(paramValueBytes))
	case reflect.Int64:
		return fmt.Sprintf("%d", int64(byteOrder.Uint64(paramValueBytes)))
	case reflect.Uint:
		return fmt.Sprintf("%d", byteOrder.Uint64(paramValueBytes))
	case reflect.Int:
		return fmt.Sprintf("%d", int(byteOrder.Uint64(paramValueBytes)))
	case reflect.Pointer:
		return fmt.Sprintf("0x%X", byteOrder.Uint64(paramValueBytes))
	case reflect.String:
		return string(paramValueBytes)
	case reflect.Bool:
		if paramValueBytes[0] == 1 {
			return "true"
		} else {
			return "false"
		}
	default:
		return "<UNSUPPORTED>"
	}
}

func isTypeWithHeader(pieceType byte) bool {
	return reflect.Kind(pieceType) == reflect.Struct ||
		reflect.Kind(pieceType) == reflect.Slice ||
		reflect.Kind(pieceType) == reflect.Array ||
		reflect.Kind(pieceType) == reflect.Pointer
}
