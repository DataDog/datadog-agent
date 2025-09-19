// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"errors"

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
	skipIndicies     []byte
}

var ddDebuggerString = jsontext.String("dd_debugger")

type ddDebuggerSource struct{}

func (ddDebuggerSource) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(ddDebuggerString)
}

// In the root data item, before the expressions, there is a bitset
// which conveys if expression values are present in the data.
// The rootType.PresenceBitsetSize conveys the size of the bitset in
// bytes, and presence bits in the bitset correspond with index of
// the expression in the root ir.
func expressionIsPresent(bitset []byte, expressionIndex int) bool {
	idx, bit := expressionIndex/8, expressionIndex%8
	return idx < len(bitset) && bitset[idx]&(1<<byte(bit)) != 0
}

// Helper functions for skipIndicies bitset operations
func isSkipped(bitset []byte, index int) bool {
	idx, bit := index/8, index%8
	return idx < len(bitset) && bitset[idx]&(1<<byte(bit)) != 0
}

func setSkipped(bitset []byte, index int) {
	idx, bit := index/8, index%8
	if idx < len(bitset) {
		bitset[idx] |= 1 << byte(bit)
	}
}

var errEvaluation = errors.New("evaluation error")

// processExpression processes a single expression from the root type expressions
func (ad *argumentsData) processExpression(
	enc *jsontext.Encoder,
	expr *ir.RootExpression,
	presenceBitSet []byte,
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
	if !expressionIsPresent(presenceBitSet, expressionIndex) && parameterSize != 0 {
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
		if isSkipped(ad.skipIndicies, i) {
			continue
		}
		if err := ad.processExpression(enc, expr, presenceBitSet, i); errors.Is(err, errEvaluation) {
			// This expression resulted in an evaluation error, we mark it to be skipped
			// and will try again
			setSkipped(ad.skipIndicies, i)
			return err
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
	if err := writeTokens(enc,
		jsontext.BeginObject,
		jsontext.String("function"),
		jsontext.String(sl.Function),
		jsontext.String("fileName"),
		jsontext.String(sl.File),
		jsontext.String("lineNumber"),
		jsontext.Int(int64(sl.Line)),
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
