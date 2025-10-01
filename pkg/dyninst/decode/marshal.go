// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
)

type logger struct {
	Name       string `json:"name"`
	Method     string `json:"method"`
	Version    int    `json:"version"`
	ThreadID   int    `json:"thread_id"`
	ThreadName string `json:"thread_name"`
}

type debuggerData struct {
	Snapshot         snapshotData `json:"snapshot"`
	EvaluationErrors []string     `json:"evaluationErrors,omitempty"`
}

type message struct {
	probe            *ir.Probe
	captureData      *captureData
	captureMap       map[ir.TypeID]*captureEvent
	evaluationErrors *[]string
}

// MarshalJSONTo is used to marshal the expression template of the user specified probe
// There are two types of segments in the template: string segments and json segments.
// String segments are string literals and are written directly, while
// JSON segments are formatted and the result is written to the encoder.
// The following logic is followed, meant to emulate the behavior of %#v string format:
//
// Integers, floats, booleans, strings: Printed as literals in Go syntax.
// Pointers: Printed as "0x" followed by the hexadecimal representation of the pointer address.
// Structs: Printed as "typeName{field1: value1, field2: value2, ...}".
// Arrays: Printed as "[numElements]typeName{element1, element2, ...}".
// Slices: Printed as "[]slice{element1, element2, ...}".
// Other types: Printed as "<type name>".
func (m *message) MarshalJSONTo(enc *jsontext.Encoder) error {
	var sb strings.Builder

	// Go through each template segment, if its a string, write it directly to the encoder, if its a json segment, process the expression
	// and write the result to the encoder.
	for _, seg := range m.probe.Template.Segments {
		switch segTyped := seg.(type) {
		case ir.JSONSegment:
			found := false
			for k, v := range m.captureMap {
				if _, ok := segTyped.RootTypeExpressionIndicies[k]; ok {
					found = true
					err := v.extractExpressionRawValue(&sb, segTyped.RootTypeExpressionIndicies[k])
					if err != nil {
						*v.evaluationErrors = append(*v.evaluationErrors, err.Error())
						continue
					}
					break
				}
			}
			if !found {
				return fmt.Errorf("no capture event found for segment %T", seg)
			}
		case ir.StringSegment:
			if _, err := sb.WriteString(segTyped.Value); err != nil {
				*m.evaluationErrors = append(*m.evaluationErrors, err.Error())
				continue
			}
		default:
			*m.evaluationErrors = append(*m.evaluationErrors, fmt.Sprintf("unsupported segment type: %T", seg))
		}
	}
	return enc.WriteToken(jsontext.String(sb.String()))
}

type snapshotData struct {
	// static fields:
	ID        uuid.UUID `json:"id"`
	Timestamp int       `json:"timestamp"`
	Language  string    `json:"language"`

	// dynamic fields:
	Stack    stackData   `json:"stack"`
	Probe    probeData   `json:"probe"`
	Captures captureData `json:"captures"`
}

type probeData struct {
	ID       string       `json:"id,omitempty"`
	Location locationData `json:"location"`
}

type locationData struct {
	Method string `json:"method,omitempty"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitzero"`
	Type   string `json:"type,omitempty"`
}

type captureData struct {
	Entry  *captureEvent    `json:"entry,omitempty"`
	Return *captureEvent    `json:"return,omitempty"`
	Lines  *lineCaptureData `json:"lines,omitempty"`
}

type lineCaptureData struct {
	sourceLine string
	capture    *captureEvent
}

func (l *lineCaptureData) clear() {
	l.sourceLine = ""
	l.capture = nil
}

func (l *lineCaptureData) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := writeTokens(enc,
		jsontext.BeginObject,
		jsontext.String(l.sourceLine)); err != nil {
		return err
	}
	if err := json.MarshalEncode(enc, l.capture); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}
	return nil

}

type captureEvent struct {
	encodingContext

	rootData         []byte
	rootType         *ir.EventRootType
	evaluationErrors *[]string
	skippedIndices   bitset
}

func (ce *captureEvent) clear() {
	ce.rootData = nil
	ce.rootType = nil
	ce.evaluationErrors = nil

	clear(ce.dataItems)
	clear(ce.currentlyEncoding)
	ce.skippedIndices.reset(0)
}

func (ce *captureEvent) init(
	ev output.Event, types map[ir.TypeID]ir.Type, evalErrors *[]string,
) error {
	var rootType *ir.EventRootType
	var rootData []byte
	for item, err := range ev.DataItems() {
		if err != nil {
			return fmt.Errorf("error getting data items: %w", err)
		}
		if rootType == nil {
			var ok bool
			rootData, ok = item.Data()
			if !ok {
				// This should never happen.
				return errors.New("root data item marked as a failed read")
			}
			rootTypeID := ir.TypeID(item.Type())
			rootType, ok = types[rootTypeID].(*ir.EventRootType)
			if !ok {
				return errors.New("expected event of type root first")
			}
			continue
		}
		key := typeAndAddr{irType: item.Type(), addr: item.Header().Address}
		ce.dataItems[key] = item
	}
	if rootType == nil {
		return errors.New("no root type found")
	}
	ce.rootType = rootType
	ce.rootData = rootData
	ce.skippedIndices.reset(len(rootType.Expressions))
	ce.evaluationErrors = evalErrors
	return nil
}

var ddDebuggerString = jsontext.String("dd_debugger")

type ddDebuggerSource struct{}

func (ddDebuggerSource) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(ddDebuggerString)
}

var errEvaluation = errors.New("evaluation error")

// processExpression processes a single expression from the root type expressions
func (ce *captureEvent) processExpression(
	enc *jsontext.Encoder,
	expr *ir.RootExpression,
	presenceBitSet bitset,
	expressionIndex int, // Index within the root type expressions
) error {
	parameterType := expr.Expression.Type
	parameterSize := parameterType.GetByteSize()
	ub := expr.Offset + parameterSize
	if int(ub) > len(ce.rootData) {
		*ce.evaluationErrors = append(
			*ce.evaluationErrors,
			"could not read parameter data from root data, length mismatch",
		)
		return errEvaluation
	}
	data := ce.rootData[expr.Offset:ub]
	if err := writeTokens(enc, jsontext.String(expr.Name)); err != nil {
		return err
	}
	if !presenceBitSet.get(expressionIndex) && parameterSize != 0 {
		// Set not capture reason.
		if err := writeTokens(enc,
			jsontext.BeginObject,
			jsontext.String("type"),
			jsontext.String(parameterType.GetName()),
			tokenNotCapturedReason,
			tokenNotCapturedReasonUnavailable,
			jsontext.EndObject,
		); err != nil {
			return err
		}
		return nil
	}
	err := encodeValue(
		&ce.encodingContext, enc, parameterType.GetID(), data, parameterType.GetName(),
	)
	if err != nil {
		*ce.evaluationErrors = append(*ce.evaluationErrors, ce.rootType.Name+err.Error())
		return errEvaluation
	}
	return nil
}

// extractExpressionRawValue extracts the raw string value of an expression without JSON encoding
func (ce *captureEvent) extractExpressionRawValue(sb *strings.Builder, expressionIndex int) error {
	if expressionIndex >= len(ce.rootType.Expressions) {
		return fmt.Errorf("expression index %d out of bounds", expressionIndex)
	}
	if ce.skippedIndices.get(expressionIndex) {
		return fmt.Errorf("expression skipped")
	}
	expr := ce.rootType.Expressions[expressionIndex]
	parameterType := expr.Expression.Type
	parameterSize := parameterType.GetByteSize()
	ub := expr.Offset + parameterSize
	if int(ub) > len(ce.rootData) {
		return fmt.Errorf("could not read parameter data from root data, length mismatch")
	}

	parameterData := ce.rootData[expr.Offset:ub]
	presenceBitSet := bitset(ce.rootData[:ce.rootType.PresenceBitsetSize])
	if !presenceBitSet.get(expressionIndex) && parameterSize != 0 {
		return fmt.Errorf("expression not captured")
	}

	// Convert binary data to raw string value based on type
	decoderType, ok := ce.encodingContext.typesByID[parameterType.GetID()]
	if !ok {
		return fmt.Errorf("no decoder type found for type %s", parameterType.GetName())
	}

	return extractRawValue(sb, decoderType, &ce.encodingContext, parameterData)
}

// extractRawValue converts binary data to raw string representation and writes it to the builder
func extractRawValue(sb *strings.Builder, dt decoderType, c *encodingContext, data []byte) error {
	switch t := dt.(type) {
	case *baseType:
		return extractBaseTypeValue(sb, t, data)
	case *goStringHeaderType:
		return extractStringValue(sb, t, c, data)
	case *pointerType:
		_, err := sb.WriteString("0x" + strconv.FormatUint(binary.NativeEndian.Uint64(data), 16))
		return err
	case *structureType:
		// Format struct fields as readable string
		return extractStructureTypeValue(sb, t, c, data)
	case *arrayType:
		// Format array elements as readable string
		return extractArrayTypeValue(sb, t, c, data)
	case *goSliceHeaderType:
		// Format slice elements as readable string
		return extractSliceTypeValue(sb, t, c, data)
	default:
		// Fallback: write type name for unsupported types
		_, err := sb.WriteString("<" + dt.irType().GetName() + ">")
		return err
	}
}

func extractStructureTypeValue(sb *strings.Builder, s *structureType, c *encodingContext, data []byte) error {
	structType := s.irType().(*ir.StructureType)
	_, err := sb.WriteString(fmt.Sprintf("%s{", structType.GetName()))
	if err != nil {
		return err
	}
	for field := range structType.Fields() {
		fieldEnd := field.Offset + field.Type.GetByteSize()
		if fieldEnd > uint32(len(data)) {
			// Field extends beyond available data - skip it
			continue
		}

		fieldData := data[field.Offset:fieldEnd]

		// Get the decoder type for this field
		fieldDecoderType, ok := c.typesByID[field.Type.GetID()]
		if !ok {
			// Unknown field type - show type name
			_, err = sb.WriteString(fmt.Sprintf("%s = <%s>", field.Name, field.Type.GetName()))
			if err != nil {
				return err
			}
			continue
		}

		// Extract the raw value for this field
		// Format as "fieldName = value"
		_, err = sb.WriteString(fmt.Sprintf(" %s = ", field.Name))
		if err != nil {
			return err
		}
		err = extractRawValue(sb, fieldDecoderType, c, fieldData)
		if err != nil {
			// Error extracting field - show error
			_, err = sb.WriteString(fmt.Sprintf("%s = <error: %v>", field.Name, err))
			if err != nil {
				return err
			}
			continue
		}

	}
	_, err = sb.WriteString(" }")
	if err != nil {
		return err
	}
	return nil
}

// extractArrayTypeValue formats array elements as a readable string: "[element1, element2, ...]"
func extractArrayTypeValue(sb *strings.Builder, a *arrayType, c *encodingContext, data []byte) error {
	arrayType := a.irType().(*ir.ArrayType)
	elementSize := int(arrayType.Element.GetByteSize())
	numElements := int(arrayType.Count)

	// Limit output for very large arrays
	maxElementsToShow := 10
	if numElements > maxElementsToShow {
		numElements = maxElementsToShow
	}

	// Get the decoder type for array elements
	elementDecoderType, ok := c.typesByID[arrayType.Element.GetID()]
	if !ok {
		_, err := sb.WriteString(fmt.Sprintf("[%d]<%s>", arrayType.Count, arrayType.Element.GetName()))
		return err
	}

	_, err := sb.WriteString(fmt.Sprintf("[%d]%s{", arrayType.Count, arrayType.Element.GetName()))
	if err != nil {
		return err
	}

	for i := 0; i < numElements; i++ {
		offset := i * elementSize
		endIdx := offset + elementSize
		if endIdx > len(data) {
			break
		}

		elementData := data[offset:endIdx]
		err = extractRawValue(sb, elementDecoderType, c, elementData)
		if err != nil {
			return err
		}
		if i < numElements-1 {
			_, err = sb.WriteString(", ")
			if err != nil {
				return err
			}
		}
	}
	_, err = sb.WriteString("}")
	return err
}

// extractSliceTypeValue formats slice elements as a readable string: "[element1, element2, ...]"
func extractSliceTypeValue(sb *strings.Builder, s *goSliceHeaderType, c *encodingContext, data []byte) error {
	sliceType := s.irType().(*ir.GoSliceHeaderType)
	if len(data) < 16 {
		return errors.New("data too short for slice")
	}
	// Extract slice header: pointer (8 bytes) + length (8 bytes) + capacity (8 bytes)
	address := binary.NativeEndian.Uint64(data[0:8])
	length := binary.NativeEndian.Uint64(data[8:16])

	if address == 0 || length == 0 {
		return errors.New("slice address or length is 0")
	}

	// Get the slice data
	sliceDataItem, ok := c.dataItems[typeAndAddr{
		irType: uint32(sliceType.Data.GetID()),
		addr:   address,
	}]
	if !ok {
		return fmt.Errorf("slice data item not found at address %#x with len %d", address, length)
	}

	sliceData, ok := sliceDataItem.Data()
	if !ok {
		return fmt.Errorf("slice data item marked as failed read")
	}

	elementSize := int(sliceType.Data.Element.GetByteSize())
	sliceLength := int(len(sliceData)) / elementSize

	// Limit output for very large slices
	maxElementsToShow := 10
	if sliceLength > maxElementsToShow {
		sliceLength = maxElementsToShow
	}

	// Get the decoder type for slice elements
	elementDecoderType, ok := c.typesByID[sliceType.Data.Element.GetID()]
	if !ok {
		return fmt.Errorf("no decoder type found for slice element type %s", sliceType.Data.Element.GetName())
	}

	_, err := sb.WriteString(fmt.Sprintf("[]%s{", sliceType.Data.Element.GetName()))
	if err != nil {
		return err
	}
	elementByteSize := int(sliceType.Data.Element.GetByteSize())
	for i := 0; i < sliceLength; i++ {
		elementData := sliceData[i*elementByteSize : (i+1)*elementByteSize]
		err := extractRawValue(sb, elementDecoderType, c, elementData)
		if err != nil {
			return fmt.Errorf("error extracting raw value for slice element %d: %w", i, err)
		}

		if i < sliceLength-1 {
			_, err = sb.WriteString(", ")
			if err != nil {
				return err
			}
		}
	}

	_, err = sb.WriteString("}")
	if err != nil {
		return err
	}

	return nil
}

// extractBaseTypeValue converts base type binary data to string
func extractBaseTypeValue(sb *strings.Builder, b *baseType, data []byte) error {
	kind, ok := (*ir.BaseType)(b).GetGoKind()
	if !ok {
		return fmt.Errorf("no go kind for type %s", (*ir.BaseType)(b).GetName())
	}
	var err error
	switch kind {
	case reflect.Bool:
		if len(data) < 1 {
			return errors.New("data too short for bool")
		}
		if data[0] == 1 {
			_, err = sb.WriteString("true")
			return err
		}
		_, err = sb.WriteString("false")
		return err
	case reflect.Int, reflect.Int64:
		if len(data) < 8 {
			return errors.New("data too short for int64")
		}
		val := int64(binary.NativeEndian.Uint64(data))
		_, err = sb.WriteString(strconv.FormatInt(val, 10))
		return err
	case reflect.Int32:
		if len(data) < 4 {
			return errors.New("data too short for int32")
		}
		val := int32(binary.NativeEndian.Uint32(data))
		_, err = sb.WriteString(strconv.FormatInt(int64(val), 10))
		return err

	case reflect.Int16:
		if len(data) < 2 {
			return errors.New("data too short for int16")
		}
		val := int16(binary.NativeEndian.Uint16(data))
		_, err = sb.WriteString(strconv.FormatInt(int64(val), 10))
		return err

	case reflect.Int8:
		if len(data) < 1 {
			return errors.New("data too short for int8")
		}
		val := int8(data[0])
		_, err = sb.WriteString(strconv.FormatInt(int64(val), 10))
		return err

	case reflect.Uint, reflect.Uint64:
		if len(data) < 8 {
			return errors.New("data too short for uint64")
		}
		val := binary.NativeEndian.Uint64(data)
		_, err = sb.WriteString(strconv.FormatUint(val, 10))
		return err

	case reflect.Uint32:
		if len(data) < 4 {
			return errors.New("data too short for uint32")
		}
		val := binary.NativeEndian.Uint32(data)
		_, err = sb.WriteString(strconv.FormatUint(uint64(val), 10))
		return err

	case reflect.Uint16:
		if len(data) < 2 {
			return errors.New("data too short for uint16")
		}
		val := binary.NativeEndian.Uint16(data)
		_, err = sb.WriteString(strconv.FormatUint(uint64(val), 10))
		return err

	case reflect.Uint8:
		if len(data) < 1 {
			return errors.New("data too short for uint8")
		}
		_, err = sb.WriteString(strconv.FormatUint(uint64(data[0]), 10))
		return err

	case reflect.Float32:
		if len(data) < 4 {
			return errors.New("data too short for float32")
		}
		bits := binary.NativeEndian.Uint32(data)
		val := math.Float32frombits(bits)
		_, err = sb.WriteString(strconv.FormatFloat(float64(val), 'g', -1, 32))
		return err

	case reflect.Float64:
		if len(data) < 8 {
			return errors.New("data too short for float64")
		}
		bits := binary.NativeEndian.Uint64(data)
		val := math.Float64frombits(bits)
		_, err = sb.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
		return err

	case reflect.String:
		// Go strings are not stored as baseType, this shouldn't happen
		_, err = sb.WriteString(string(data))
		return err

	default:
		return fmt.Errorf("unsupported base type kind: %v", kind)
	}
}

// extractStringValue extracts string value from Go string header
func extractStringValue(sb *strings.Builder, s *goStringHeaderType, c *encodingContext, data []byte) error {
	fieldEnd := s.strFieldOffset + uint32(s.strFieldSize)
	if fieldEnd >= uint32(len(data)) {
		return nil
	}
	strLen := binary.NativeEndian.Uint64(data[s.lenFieldOffset : s.lenFieldOffset+uint32(s.lenFieldSize)])
	address := binary.NativeEndian.Uint64(data[s.strFieldOffset : s.strFieldOffset+uint32(s.strFieldSize)])
	if address == 0 || strLen == 0 {
		return nil
	}
	stringValue, ok := c.dataItems[typeAndAddr{
		irType: uint32(s.GoStringHeaderType.Data.GetID()),
		addr:   address,
	}]
	if !ok {
		return nil
	}
	stringData, ok := stringValue.Data()
	if !ok {
		return nil
	}
	if len(stringData) < int(strLen) {
		return nil
	}

	_, err := sb.WriteString(string(stringData[:strLen]))
	return err
}

func (ce *captureEvent) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}

	if ce.rootType.PresenceBitsetSize > uint32(len(ce.rootData)) {
		return errors.New("presence bitset is out of bounds")
	}
	presenceBitSet := ce.rootData[:ce.rootType.PresenceBitsetSize]

	for _, kind := range []struct {
		kind  ir.RootExpressionKind
		token jsontext.Token
	}{
		{kind: ir.RootExpressionKindArgument, token: jsontext.String("arguments")},
		{kind: ir.RootExpressionKindLocal, token: jsontext.String("locals")},
	} {
		// We iterate over the 'Expressions' of the EventRoot which contains
		// metadata and raw bytes of the parameters of this function.
		var haveKind bool
		for i, expr := range ce.rootType.Expressions {
			if expr.Kind != kind.kind {
				continue
			}
			if ce.skippedIndices.get(i) {
				continue
			}
			if !haveKind {
				haveKind = true
				if err := writeTokens(enc, kind.token, jsontext.BeginObject); err != nil {
					return err
				}
			}
			err := ce.processExpression(enc, expr, presenceBitSet, i)
			if errors.Is(err, errEvaluation) {
				// This expression resulted in an evaluation error, we mark it to be
				// skipped and will try again
				ce.skippedIndices.set(i)
			}
			if err != nil {
				return err
			}
		}
		if haveKind {
			if err := writeTokens(enc, jsontext.EndObject); err != nil {
				return err
			}
		}

	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}
	return nil
}

type stackData struct {
	frames []symbol.StackFrame
}

func (sd *stackData) MarshalJSONTo(enc *jsontext.Encoder) error {
	var err error
	if err = writeTokens(enc, jsontext.BeginArray); err != nil {
		return err
	}

	for i := range sd.frames {
		for j := range sd.frames[i].Lines {
			if err = json.MarshalEncode(
				enc, (*stackLine)(&sd.frames[i].Lines[j]),
			); err != nil {
				return err
			}
		}
	}
	if err = writeTokens(enc, jsontext.EndArray); err != nil {
		return err
	}
	return nil
}

type stackLine gosym.GoLocation

func (sl *stackLine) MarshalJSONTo(enc *jsontext.Encoder) error {
	goLoc := (*gosym.GoLocation)(sl)
	if err := writeTokens(enc,
		jsontext.BeginObject,
		jsontext.String("function"),
		jsontext.String(goLoc.Function),
		jsontext.String("fileName"),
		jsontext.String(goLoc.File),
		jsontext.String("lineNumber"),
		jsontext.Int(int64(goLoc.Line)),
		jsontext.EndObject,
	); err != nil {
		return err
	}
	return nil
}

func encodeValue(
	c *encodingContext,
	enc *jsontext.Encoder,
	typeID ir.TypeID,
	data []byte,
	valueType string,
) error {
	decoderType, ok := c.getType(typeID)
	if !ok {
		return errors.New("no decoder type found")
	}
	if err := writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.String("type"), jsontext.String(valueType)); err != nil {
		return err
	}
	if err := decoderType.encodeValueFields(c, enc, data); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.EndObject); err != nil {
		return err
	}
	return nil
}

func writeTokens(enc *jsontext.Encoder, tokens ...jsontext.Token) error {
	for i := range tokens {
		err := enc.WriteToken(tokens[i])
		if err != nil {
			return err
		}
	}
	return nil
}
