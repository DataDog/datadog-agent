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
	probe         *ir.Probe
	argumentsData *argumentsData
}

func (m *message) MarshalJSONTo(enc *jsontext.Encoder) error {
	sb := strings.Builder{}

	// Go through each template segment, if its a string, write it directly to the encoder, if its a json segment, process the expression
	// and write the result to the encoder.
	for _, seg := range m.probe.Segments {
		switch segTyped := seg.(type) {
		case ir.JSONSegment:
			// Extract raw value from expression (no JSON encoding)
			value, err := m.argumentsData.extractExpressionRawValue(segTyped.ExpressionIndex)
			if err != nil {
				return err
			}
			sb.WriteString(value)
		case ir.StringSegment:
			if _, err := sb.WriteString(segTyped.Value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported segment type: %T", seg)
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
	Entry capturePointData `json:"entry"`
}

type capturePointData struct {
	Arguments argumentsData `json:"arguments"`
}

type argumentsData struct {
	rootData         []byte
	rootType         *ir.EventRootType
	event            Event
	decoder          *Decoder
	evaluationErrors *[]string
	skippedIndices   *bitset
}

var ddDebuggerString = jsontext.String("dd_debugger")

type ddDebuggerSource struct{}

func (ddDebuggerSource) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(ddDebuggerString)
}

var errEvaluation = errors.New("evaluation error")

// processExpression processes a single expression from the root type expressions
func (ad *argumentsData) processExpression(
	enc *jsontext.Encoder,
	expr *ir.RootExpression,
	presenceBitSet bitset,
	expressionIndex int,
) error {
	parameterType := expr.Expression.Type
	parameterSize := parameterType.GetByteSize()
	ub := expr.Offset + parameterSize
	if int(ub) > len(ad.rootData) {
		*ad.evaluationErrors = append(*ad.evaluationErrors, "could not read parameter data from root data, length mismatch")
		return errEvaluation
	}
	parameterData := ad.rootData[expr.Offset:ub]
	if err := writeTokens(enc, jsontext.String(expr.Name)); err != nil {
		return err
	}
	if !presenceBitSet.get(expressionIndex) && parameterSize != 0 {
		// Set not capture reason
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
	err := ad.decoder.encodeValue(enc,
		parameterType.GetID(),
		parameterData,
		parameterType.GetName(),
	)
	if err != nil {
		*ad.evaluationErrors = append(*ad.evaluationErrors, ad.rootType.Name+err.Error())
		return errEvaluation
	}
	return nil
}

// extractExpressionRawValue extracts the raw string value of an expression without JSON encoding
func (ad *argumentsData) extractExpressionRawValue(expressionIndex int) (string, error) {
	if expressionIndex >= len(ad.rootType.Expressions) {
		return "", fmt.Errorf("expression index %d out of bounds", expressionIndex)
	}

	expr := ad.rootType.Expressions[expressionIndex]
	parameterType := expr.Expression.Type
	parameterSize := parameterType.GetByteSize()
	ub := expr.Offset + parameterSize

	if int(ub) > len(ad.rootData) {
		return "", fmt.Errorf("could not read parameter data from root data, length mismatch")
	}

	parameterData := ad.rootData[expr.Offset:ub]

	// Check if expression is present using presence bitset
	presenceBitSet := bitset(ad.rootData[:ad.rootType.PresenceBitsetSize])
	if !presenceBitSet.get(expressionIndex) && parameterSize != 0 {
		return "", fmt.Errorf("expression not captured")
	}

	// Convert binary data to raw string value based on type
	decoderType, ok := ad.decoder.decoderTypes[parameterType.GetID()]
	if !ok {
		return "", fmt.Errorf("no decoder type found for type %s", parameterType.GetName())
	}

	return extractRawValue(decoderType, ad.decoder, parameterData)
}

// extractRawValue converts binary data to raw string representation
func extractRawValue(dt decoderType, d *Decoder, data []byte) (string, error) {
	switch t := dt.(type) {
	case *baseType:
		return extractBaseTypeValue(t, data)
	case *goStringHeaderType:
		return extractStringValue(t, d, data)
	case *pointerType:
		// For template values, we typically want the pointed-to value
		return "0x" + strconv.FormatUint(binary.NativeEndian.Uint64(data), 16), nil
	case *structureType:
		// Complex types might need special handling - for now return type name
		return "<" + dt.irType().GetName() + ">", nil
	default:
		// Fallback: return type name for unsupported types
		return "<" + dt.irType().GetName() + ">", nil
	}
}

// extractBaseTypeValue converts base type binary data to string
func extractBaseTypeValue(b *baseType, data []byte) (string, error) {
	kind, ok := (*ir.BaseType)(b).GetGoKind()
	if !ok {
		return "", fmt.Errorf("no go kind for type %s", (*ir.BaseType)(b).GetName())
	}

	switch kind {
	case reflect.Bool:
		if len(data) < 1 {
			return "", errors.New("data too short for bool")
		}
		if data[0] == 1 {
			return "true", nil
		}
		return "false", nil

	case reflect.Int, reflect.Int64:
		if len(data) < 8 {
			return "", errors.New("data too short for int64")
		}
		val := int64(binary.NativeEndian.Uint64(data))
		return strconv.FormatInt(val, 10), nil

	case reflect.Int32:
		if len(data) < 4 {
			return "", errors.New("data too short for int32")
		}
		val := int32(binary.NativeEndian.Uint32(data))
		return strconv.FormatInt(int64(val), 10), nil

	case reflect.Int16:
		if len(data) < 2 {
			return "", errors.New("data too short for int16")
		}
		val := int16(binary.NativeEndian.Uint16(data))
		return strconv.FormatInt(int64(val), 10), nil

	case reflect.Int8:
		if len(data) < 1 {
			return "", errors.New("data too short for int8")
		}
		val := int8(data[0])
		return strconv.FormatInt(int64(val), 10), nil

	case reflect.Uint, reflect.Uint64:
		if len(data) < 8 {
			return "", errors.New("data too short for uint64")
		}
		val := binary.NativeEndian.Uint64(data)
		return strconv.FormatUint(val, 10), nil

	case reflect.Uint32:
		if len(data) < 4 {
			return "", errors.New("data too short for uint32")
		}
		val := binary.NativeEndian.Uint32(data)
		return strconv.FormatUint(uint64(val), 10), nil

	case reflect.Uint16:
		if len(data) < 2 {
			return "", errors.New("data too short for uint16")
		}
		val := binary.NativeEndian.Uint16(data)
		return strconv.FormatUint(uint64(val), 10), nil

	case reflect.Uint8:
		if len(data) < 1 {
			return "", errors.New("data too short for uint8")
		}
		return strconv.FormatUint(uint64(data[0]), 10), nil

	case reflect.Float32:
		if len(data) < 4 {
			return "", errors.New("data too short for float32")
		}
		bits := binary.NativeEndian.Uint32(data)
		val := math.Float32frombits(bits)
		return strconv.FormatFloat(float64(val), 'g', -1, 32), nil

	case reflect.Float64:
		if len(data) < 8 {
			return "", errors.New("data too short for float64")
		}
		bits := binary.NativeEndian.Uint64(data)
		val := math.Float64frombits(bits)
		return strconv.FormatFloat(val, 'g', -1, 64), nil

	case reflect.String:
		// Go strings are not stored as baseType, this shouldn't happen
		return string(data), nil

	default:
		return "", fmt.Errorf("unsupported base type kind: %v", kind)
	}
}

// extractStringValue extracts string value from Go string header
func extractStringValue(s *goStringHeaderType, d *Decoder, data []byte) (string, error) {
	fieldEnd := s.strFieldOffset + uint32(s.strFieldSize)
	if fieldEnd >= uint32(len(data)) {
		return "", nil
	}
	strLen := binary.NativeEndian.Uint64(data[s.lenFieldOffset : s.lenFieldOffset+uint32(s.lenFieldSize)])
	address := binary.NativeEndian.Uint64(data[s.strFieldOffset : s.strFieldOffset+uint32(s.strFieldSize)])
	if address == 0 || strLen == 0 {
		return "", nil
	}
	stringValue, ok := d.dataItems[typeAndAddr{
		irType: uint32(s.GoStringHeaderType.Data.GetID()),
		addr:   address,
	}]
	if !ok {
		return "", nil
	}
	stringData, ok := stringValue.Data()
	if !ok {
		return "", nil
	}
	if len(stringData) < int(strLen) {
		return "", nil
	}

	return string(stringData[:strLen]), nil
}

func (ad *argumentsData) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}

	if ad.rootType.PresenceBitsetSize > uint32(len(ad.rootData)) {
		return errors.New("presence bitset is out of bounds")
	}
	presenceBitSet := ad.rootData[:ad.rootType.PresenceBitsetSize]
	// We iterate over the 'Expressions' of the EventRoot which contains
	// metadata and raw bytes of the parameters of this function.
	for i, expr := range ad.rootType.Expressions {
		if ad.skippedIndices.get(i) {
			continue
		}
		if err := ad.processExpression(enc, expr, presenceBitSet, i); errors.Is(err, errEvaluation) {
			// This expression resulted in an evaluation error, we mark it to be skipped
			// and will try again
			ad.skippedIndices.set(i)
			return errEvaluation
		} else if err != nil {
			return err
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

func (d *Decoder) encodeValue(
	enc *jsontext.Encoder,
	typeID ir.TypeID,
	data []byte,
	valueType string,
) error {
	decoderType, ok := d.decoderTypes[typeID]
	if !ok {
		return errors.New("no decoder type found")
	}
	if err := writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}
	if err := writeTokens(enc, jsontext.String("type"), jsontext.String(valueType)); err != nil {
		return err
	}
	if err := decoderType.encodeValueFields(d, enc, data); err != nil {
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
