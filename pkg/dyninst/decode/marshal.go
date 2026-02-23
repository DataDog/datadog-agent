// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type logger struct {
	Name       string `json:"name"`
	Method     string `json:"method"`
	Version    int    `json:"version"`
	ThreadID   int    `json:"thread_id"`
	ThreadName string `json:"thread_name"`
}

type debuggerData struct {
	Snapshot         snapshotData      `json:"snapshot,omitempty"`
	EvaluationErrors []evaluationError `json:"evaluationErrors,omitempty"`
}

type messageData struct {
	duration              *uint64
	durationMissingReason *string
	entryOrLine           *captureEvent
	_return               *captureEvent
	template              *ir.Template
}

func (m *messageData) MarshalJSONTo(enc *jsontext.Encoder) error {
	var result bytes.Buffer
	limits := &formatLimits{
		maxBytes:           maxLogLineBytes,
		maxCollectionItems: maxLogCollectionItems,
		maxFields:          maxLogFieldCount,
	}

	for _, segment := range m.template.Segments {
		// Check if we've exceeded the total byte limit.
		if result.Len() >= maxLogLineBytes {
			break
		}

		switch seg := segment.(type) {
		case ir.StringSegment:
			// Literal string - append directly, but check limits.
			segStr := string(seg)
			remainingBytes := maxLogLineBytes - result.Len()
			if len(segStr) > remainingBytes {
				segStr = segStr[:remainingBytes]
			}
			result.WriteString(segStr)
		case *ir.JSONSegment:
			savedLen := result.Len()
			// Update limits to reflect remaining bytes.
			limits.maxBytes = maxLogLineBytes - savedLen
			if err := m.processJSONSegment(&result, seg, limits); err != nil {
				// Reset buffer to saved length and write error.
				result.Truncate(savedLen)
				limits.maxBytes = maxLogLineBytes - savedLen
				writeBoundedError(&result, limits, "error", err.Error())
			}
			// Update limits after processing segment.
			limits.maxBytes = maxLogLineBytes - result.Len()
		case ir.InvalidSegment:
			writeBoundedError(&result, limits, "error", seg.Error)
		case *ir.DurationSegment:
			if m.duration == nil {
				if m.durationMissingReason != nil {
					writeBoundedError(&result, limits, "error", *m.durationMissingReason)
				} else {
					writeBoundedError(&result, limits, "error", "@duration is not available")
				}
			} else {
				n, _ := fmt.Fprintf(&result, "%f", time.Duration(*m.duration).Seconds()*1000)
				limits.consume(n)
			}

		default:
			return fmt.Errorf(
				"unexpected segment type: %T: %+#v", seg, seg,
			)
		}
	}
	return writeTokens(enc, jsontext.String(result.String()))
}

func (m *messageData) processJSONSegment(
	result *bytes.Buffer,
	seg *ir.JSONSegment,
	limits *formatLimits,
) error {
	// Get encodingContext and root data from appropriate capture event.
	var ev *captureEvent

	switch seg.EventKind {
	case ir.EventKindEntry, ir.EventKindLine:
		ev = m.entryOrLine
	case ir.EventKindReturn:
		ev = m._return
	default:
		return fmt.Errorf(
			"unexpected event kind: %v", seg.EventKind,
		)
	}

	if ev == nil || ev.rootType == nil || ev.rootData == nil {
		if !limits.canWrite(len(formatUnavailable)) {
			return nil
		}
		result.WriteString(formatUnavailable)
		limits.consume(len(formatUnavailable))
		return nil
	}

	// Expression reference - format the captured value.
	exprIdx := seg.EventExpressionIndex
	if exprIdx >= len(ev.rootType.Expressions) {
		if !limits.canWrite(len(formatUnavailable)) {
			return nil
		}
		result.WriteString(formatUnavailable)
		limits.consume(len(formatUnavailable))
		return nil
	}
	expr := ev.rootType.Expressions[exprIdx]

	// Check presence bit using same logic as processExpression.
	presenceBitsetSize := ev.rootType.PresenceBitsetSize
	if int(presenceBitsetSize) > len(ev.rootData) {
		return errors.New("presence bitset is out of bounds")
	}
	presenceBitSet := bitset(ev.rootData[:presenceBitsetSize])
	if exprIdx >= int(presenceBitsetSize)*8 {
		return errors.New("expression index out of bounds")
	}
	if !presenceBitSet.get(exprIdx) {
		// Expression evaluation failed.
		if !limits.canWrite(len(formatUnavailable)) {
			return nil
		}
		result.WriteString(formatUnavailable)
		limits.consume(len(formatUnavailable))
		return nil
	}

	// Get expression data.
	exprDataStart := expr.Offset
	exprDataEnd := exprDataStart + expr.Expression.Type.GetByteSize()
	if exprDataEnd > uint32(len(ev.rootData)) {
		return errors.New("expression data out of bounds")
	}
	exprData := ev.rootData[exprDataStart:exprDataEnd]

	// Format the value based on type using encodingContext.
	// formatType already consumes bytes internally, so we don't need to
	// track here.
	if err := formatType(
		&ev.encodingContext, result, expr.Expression.Type, exprData, limits,
	); err != nil {
		return fmt.Errorf("error formatting expression: %w", err)
	}

	return nil
}

type evaluationError struct {
	Expression string `json:"expr"`
	Message    string `json:"message"`
}

type snapshotData struct {
	// static fields:
	ID        uuid.UUID `json:"id"`
	Timestamp int       `json:"timestamp"`
	Language  string    `json:"language"`

	// dynamic fields:
	Stack    *stackData   `json:"stack,omitempty"`
	Probe    probeData    `json:"probe"`
	Captures *captureData `json:"captures,omitempty"`

	stack    stackData
	captures captureData
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
	evaluationErrors *[]evaluationError
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

var dataItemDecodingLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

func (ce *captureEvent) init(
	ev output.Event, types map[ir.TypeID]ir.Type, evalErrors *[]evaluationError,
) error {
	var rootType *ir.EventRootType
	var rootData []byte
	for item, err := range ev.DataItems() {
		if err != nil {
			if rootType == nil {
				return fmt.Errorf("error getting first data item: %w", err)
			}
			// If we have trouble decoding a data item, we still want to try
			// to emit a message. We shouldn't have this problem, but we
			// don't know why it happens and it's better to log about it than
			// to bail out completely.
			if dataItemDecodingLogLimiter.Allow() {
				log.Errorf("error getting data items (%d): %v", len(ce.dataItems), err)
			} else {
				log.Tracef("error getting data items (%d): %v", len(ce.dataItems), err)
			}
			break
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
		// We may capture dynamically sized objects multiple times with different lengths.
		// Here we just pick the most data we have, decoder will look at relevant prefix.
		prev, exists := ce.dataItems[key]
		if !exists || prev.Header().Length < item.Header().Length {
			ce.dataItems[key] = item
		}
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
			evaluationError{
				Expression: ce.rootType.Name,
				Message:    "could not read parameter data from root data, length mismatch",
			},
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
		*ce.evaluationErrors = append(*ce.evaluationErrors, evaluationError{
			Expression: ce.rootType.Name,
			Message:    err.Error(),
		})
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
		{kind: ir.RootExpressionKindCaptureExpression, token: jsontext.String("captureExpressions")},
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
				if err := writeTokens(
					enc, kind.token, jsontext.BeginObject,
				); err != nil {
					return err
				}
			}
			err := ce.processExpression(enc, expr, presenceBitSet, i)
			if errors.Is(err, errEvaluation) {
				// This expression resulted in an evaluation error, we mark it
				// to be skipped and will try again
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
	if err := writeTokens(enc, jsontext.BeginArray); err != nil {
		return err
	}
	for i := range sd.frames {
		for j := range sd.frames[i].Lines {
			sl := sd.frames[i].Lines[j]
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
		}
	}
	if err := writeTokens(enc, jsontext.EndArray); err != nil {
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
	if err := writeTokens(
		enc, jsontext.String("type"), jsontext.String(valueType),
	); err != nil {
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
