// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"errors"
	"fmt"

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
	Entry  *captureEvent `json:"entry,omitempty"`
	Return *captureEvent `json:"return,omitempty"`
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
	expressionIndex int,
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

func (ce *captureEvent) MarshalJSONTo(enc *jsontext.Encoder) error {
	if ce.rootType.PresenceBitsetSize > uint32(len(ce.rootData)) {
		return errors.New("presence bitset is out of bounds")
	}
	presenceBitSet := ce.rootData[:ce.rootType.PresenceBitsetSize]

	if err := writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}
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
