// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
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

type payloadType string

const (
	payloadTypeSnapshot payloadType = "snapshot"
)

type debuggerData struct {
	Snapshot snapshotData `json:"snapshot,omitempty"`
	Type     payloadType  `json:"type,omitempty"`
}

type messageData struct {
	entryOrLine *captureEvent
	_return     *captureEvent
	template    *ir.Template
	// returnMissingReason is non-empty when the probe expected a return
	// event but none arrived, carrying the human-readable pairing-failure
	// reason. Used to produce a specific message when a template segment
	// references @duration in that case.
	returnMissingReason string
}

// isDurationRef reports whether a parsed expression is a bare {"ref":"@duration"}.
func isDurationRef(e exprlang.Expr) bool {
	ref, ok := e.(*exprlang.RefExpr)
	return ok && ref.Ref == "@duration"
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
		// If the missing event is the return event and the segment
		// references @duration, surface the specific pairing-failure
		// reason instead of the generic "UNAVAILABLE" marker.
		if seg.EventKind == ir.EventKindReturn && isDurationRef(seg.JSON) &&
			m.returnMissingReason != "" {
			msg := "@duration is not available: " + m.returnMissingReason
			if !limits.canWrite(len(msg)) {
				return nil
			}
			result.WriteString(msg)
			limits.consume(len(msg))
			return nil
		}
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
	if expr.Redacted {
		if !limits.canWrite(len(formatRedacted)) {
			return nil
		}
		result.WriteString(formatRedacted)
		limits.consume(len(formatRedacted))
		return nil
	}

	// Check expression status.
	statusArraySize := ev.rootType.ExprStatusArraySize
	if int(statusArraySize) > len(ev.rootData) {
		return errors.New("expression status array out of bounds")
	}
	statusArray := bitset(ev.rootData[:statusArraySize])
	switch statusArray.getExprStatus(exprIdx) {
	case ir.ExprStatusPresent, ir.ExprStatusTruncated:
		// Success — fall through to format the value. Truncated is
		// treated like Present here; the filter type's formatter
		// reads currentExpr.status to surface the truncation
		// metadata where it matters.
	case ir.ExprStatusNilDeref:
		return errNilPointerEvaluating
	case ir.ExprStatusOOB:
		return errIndexOutOfBounds
	default: // ExprStatusAbsent
		if _, ok := expr.Expression.Type.(*ir.DurationType); ok {
			msg := "@duration is not available: " + ir.ErrDurationNotOnReturn
			if !limits.canWrite(len(msg)) {
				return nil
			}
			result.WriteString(msg)
			limits.consume(len(msg))
			return nil
		}
		if !limits.canWrite(len(formatUnavailable)) {
			return nil
		}
		result.WriteString(formatUnavailable)
		limits.consume(len(formatUnavailable))
		return nil
	}

	// Resolve the type for formatting — use concrete type if dict resolution
	// succeeded, otherwise fall back to the shape type.
	exprType := expr.Expression.Type
	if resolvedType, _, ok := ev.resolveDictType(expr.DictIndex); ok && resolvedType != nil {
		exprType = resolvedType
	}

	// Get expression data.
	exprDataStart := expr.Offset
	exprDataEnd := exprDataStart + exprType.GetByteSize()
	if exprDataEnd > uint32(len(ev.rootData)) {
		return errors.New("expression data out of bounds")
	}
	exprData := ev.rootData[exprDataStart:exprDataEnd]

	// Set currentExpr so type-specific formatters (notably the filter
	// types) can surface ExprStatusTruncated as collection-truncation
	// metadata. Other formatters ignore the field.
	ev.encodingContext.currentExpr.index = int(exprIdx)
	ev.encodingContext.currentExpr.status = statusArray.getExprStatus(exprIdx)

	// Format the value based on type using encodingContext.
	// formatType already consumes bytes internally, so we don't need to
	// track here.
	if err := formatType(
		&ev.encodingContext, result, exprType, exprData, limits,
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
	Stack            *stackData        `json:"stack,omitempty"`
	Probe            probeData         `json:"probe"`
	Captures         *captureData      `json:"captures,omitempty"`
	EvaluationErrors []evaluationError `json:"evaluationErrors,omitempty"`

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
	traceContext     traceContext
	evaluationErrors *[]evaluationError
	skippedIndices   bitset
}

type traceContext struct {
	traceIDLower uint64
	traceIDUpper uint64
	spanID       uint64
	parentID     uint64
	valid        bool
}

// resolveDictType checks if a variable's type can be resolved via the runtime
// dictionary. Returns (concreteType, name, true) on success. If concreteType
// is non-nil, it's an IR type from the catalog that should be used for
// decoding (matching what the eBPF used). If concreteType is nil but name is
// non-empty, only the display name was resolved (via gotype fallback) and the
// shape type should still be used for decoding.
func (ce *captureEvent) resolveDictType(dictIndex int) (ir.Type, string, bool) {
	if ce.rootType == nil || len(ce.rootType.DictEntries) == 0 || dictIndex < 0 {
		return nil, "", false
	}
	for _, de := range ce.rootType.DictEntries {
		if de.DictIndex != dictIndex {
			continue
		}
		off := de.Offset
		if int(off)+8 > len(ce.rootData) {
			return nil, "", false
		}
		runtimeType := binary.NativeEndian.Uint64(ce.rootData[off : off+8])
		if runtimeType == 0 || runtimeType == ^uint64(0) {
			return nil, "", false
		}
		// Try to resolve to a full IR type in the catalog.
		if typeID, ok := ce.getTypeIDByGoRuntimeType(uint32(runtimeType)); ok {
			if t, ok := ce.getType(typeID); ok {
				irType := t.irType()
				return irType, irType.GetName(), true
			}
		}
		// Fallback: resolve name only from gotype.
		if name, err := ce.ResolveTypeName(gotype.TypeID(runtimeType)); err == nil {
			return nil, name, true
		}
		return nil, "", false
	}
	return nil, "", false
}

func (ce *captureEvent) clear() {
	ce.rootData = nil
	ce.rootType = nil
	ce.traceContext = traceContext{}
	ce.evaluationErrors = nil

	clear(ce.dataItems)
	clear(ce.currentlyEncoding)
	ce.skippedIndices.reset(0)
}

var dataItemDecodingLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

func (ce *captureEvent) init(
	fe output.FragmentedEvent, types map[ir.TypeID]ir.Type, evalErrors *[]evaluationError,
) error {
	var rootType *ir.EventRootType
	var rootData []byte
	for ev := range fe.Fragments() {
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
			// First-valid-wins: the first synthetic trace_context data item
			// with valid=1 sets ce.traceContext. Subsequent items don't
			// overwrite, so the message-top-level dd.* fields reflect the
			// earliest captured Context that resolved to an active span.
			if _, isTraceCtx := types[ir.TypeID(item.Type())].(*ir.TraceContextType); isTraceCtx && !ce.traceContext.valid {
				if tc, ok := parseTraceContextDataItem(item); ok {
					ce.traceContext = tc
				}
			}
		}
	}
	if rootType == nil {
		return errors.New("no root type found")
	}
	ce.rootType = rootType
	ce.rootData = rootData
	ce.skippedIndices.reset(len(rootType.Expressions))
	ce.evaluationErrors = evalErrors

	// Pre-scan expression statuses for error conditions. Mark those
	// expressions as skipped and record evaluation errors up front so the
	// serialization loop never needs to restart for them.
	if rootType.ExprStatusArraySize > 0 && int(rootType.ExprStatusArraySize) <= len(rootData) {
		statusArray := bitset(rootData[:rootType.ExprStatusArraySize])
		for i, expr := range rootType.Expressions {
			switch statusArray.getExprStatus(i) {
			case ir.ExprStatusNilDeref:
				ce.skippedIndices.set(i)
				*ce.evaluationErrors = append(*ce.evaluationErrors, evaluationError{
					Expression: expr.Name,
					Message:    errNilPointerEvaluating.Error(),
				})
			case ir.ExprStatusOOB:
				ce.skippedIndices.set(i)
				*ce.evaluationErrors = append(*ce.evaluationErrors, evaluationError{
					Expression: expr.Name,
					Message:    errIndexOutOfBounds.Error(),
				})
			case ir.ExprStatusAbsent:
				// @duration is the only expression type where absent status
				// has a specific, user-meaningful reason (the BPF program
				// emits it only when entry_ktime_ns equals the probe's own
				// start_ns — i.e. the probe is not on a return).
				if _, ok := expr.Expression.Type.(*ir.DurationType); ok {
					*ce.evaluationErrors = append(*ce.evaluationErrors, evaluationError{
						Expression: expr.Name,
						Message:    ir.ErrDurationNotOnReturn,
					})
				}
			}
		}
	}

	return nil
}

// parseTraceContextDataItem parses the first ir.TraceContextByteSize bytes
// of a synthetic trace-context data item's payload as a trace_context_t.
// Returns ok=false if the data item is too short or has valid=0.
func parseTraceContextDataItem(item output.DataItem) (traceContext, bool) {
	data, ok := item.Data()
	if !ok || uint32(len(data)) < ir.TraceContextByteSize {
		return traceContext{}, false
	}
	if data[32] == 0 {
		return traceContext{}, false
	}
	return traceContext{
		traceIDLower: binary.LittleEndian.Uint64(data[0:8]),
		traceIDUpper: binary.LittleEndian.Uint64(data[8:16]),
		spanID:       binary.LittleEndian.Uint64(data[16:24]),
		parentID:     binary.LittleEndian.Uint64(data[24:32]),
		valid:        true,
	}, true
}

var ddDebuggerString = jsontext.String("dd_debugger")

type ddDebuggerSource struct{}

func (ddDebuggerSource) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(ddDebuggerString)
}

var errEvaluation = errors.New("evaluation error")
var errNilPointerEvaluating = errors.New("nil pointer dereference")
var errIndexOutOfBounds = errors.New("index out of bounds")

// processExpression processes a single expression from the root type expressions
func (ce *captureEvent) processExpression(
	enc *jsontext.Encoder,
	expr *ir.RootExpression,
	statusArray bitset,
	expressionIndex int,
) error {
	parameterType := expr.Expression.Type
	parameterSize := parameterType.GetByteSize()
	// For generic shape types, try to resolve the concrete type from the
	// runtime dictionary. If the concrete type is in our catalog, use it
	// for both the type name AND the type ID (so that data items enqueued
	// by the concrete ProcessType in eBPF match what the decoder expects).
	// If the concrete type is not in the catalog, fall back to the shape
	// type for decoding but still use the resolved name for display.
	typeName := parameterType.GetName()
	if resolvedType, resolvedName, ok := ce.resolveDictType(expr.DictIndex); ok {
		typeName = resolvedName
		if resolvedType != nil {
			// Concrete type is in the catalog — use it for decoding too.
			parameterType = resolvedType
		}
	}
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
	if expr.Redacted {
		return writeRedacted(enc, typeName, tokenNotCapturedReasonRedactedIdent)
	}
	exprStatus := statusArray.getExprStatus(expressionIndex)
	// ExprStatusPresent and ExprStatusTruncated both indicate the value
	// is present; Truncated additionally signals that a filter result
	// hit its collection cap. The filter type's decoder reads
	// currentExpr.status (set below) and appends the truncation
	// metadata to its JSON output.
	if exprStatus != ir.ExprStatusPresent &&
		exprStatus != ir.ExprStatusTruncated &&
		parameterSize != 0 {
		// Nil-deref and OOB expressions are already handled in init() and
		// marked as skipped, so we only reach here for genuinely unavailable data.
		if err := writeTokens(enc,
			jsontext.BeginObject,
			jsontext.String("type"),
			jsontext.String(typeName),
			tokenNotCapturedReason,
			tokenNotCapturedReasonUnavailable,
			jsontext.EndObject,
		); err != nil {
			return err
		}
		return nil
	}
	ce.encodingContext.currentExpr.index = expressionIndex
	ce.encodingContext.currentExpr.status = exprStatus
	err := encodeValue(
		&ce.encodingContext, enc, parameterType.GetID(), data, typeName,
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
	if ce.rootType.ExprStatusArraySize > uint32(len(ce.rootData)) {
		return errors.New("expression status array out of bounds")
	}
	statusArray := bitset(ce.rootData[:ce.rootType.ExprStatusArraySize])

	if err := writeTokens(enc, jsontext.BeginObject); err != nil {
		return err
	}
	// emitGroup writes all expressions of the given kind under the given
	// JSON key ("arguments", "captureExpressions"). Returns true if any
	// expressions were emitted.
	emitGroup := func(token jsontext.Token, kind ir.RootExpressionKind) (bool, error) {
		var have bool
		for i, expr := range ce.rootType.Expressions {
			if expr.Kind != kind {
				continue
			}
			if ce.skippedIndices.get(i) {
				continue
			}
			if !have {
				have = true
				if err := writeTokens(enc, token, jsontext.BeginObject); err != nil {
					return have, err
				}
			}
			err := ce.processExpression(enc, expr, statusArray, i)
			if errors.Is(err, errEvaluation) {
				ce.skippedIndices.set(i)
			}
			if err != nil {
				return have, err
			}
		}
		if have {
			if err := writeTokens(enc, jsontext.EndObject); err != nil {
				return have, err
			}
		}
		return have, nil
	}

	// Arguments.
	if _, err := emitGroup(jsontext.String("arguments"), ir.RootExpressionKindArgument); err != nil {
		return err
	}

	// Locals and return values share a single "locals" JSON key.
	{
		var haveLocals bool
		openLocals := func() error {
			if !haveLocals {
				haveLocals = true
				return writeTokens(enc, jsontext.String("locals"), jsontext.BeginObject)
			}
			return nil
		}

		// Emit regular locals.
		for i, expr := range ce.rootType.Expressions {
			if expr.Kind != ir.RootExpressionKindLocal {
				continue
			}
			if ce.skippedIndices.get(i) {
				continue
			}
			if err := openLocals(); err != nil {
				return err
			}
			err := ce.processExpression(enc, expr, statusArray, i)
			if errors.Is(err, errEvaluation) {
				ce.skippedIndices.set(i)
			}
			if err != nil {
				return err
			}
		}

		// Count return expressions to decide single vs multi wrapping.
		returnCount := 0
		for _, expr := range ce.rootType.Expressions {
			if expr.Kind == ir.RootExpressionKindReturn {
				returnCount++
			}
		}
		multiReturn := returnCount > 1

		// Emit return values with @return wrapping.
		var haveReturn bool
		for i, expr := range ce.rootType.Expressions {
			if expr.Kind != ir.RootExpressionKindReturn {
				continue
			}
			if ce.skippedIndices.get(i) {
				continue
			}
			if err := openLocals(); err != nil {
				return err
			}
			if !haveReturn {
				haveReturn = true
				if multiReturn {
					if err := writeTokens(enc,
						jsontext.String("@return"), jsontext.BeginObject,
						jsontext.String("fields"), jsontext.BeginObject,
					); err != nil {
						return err
					}
				}
			}
			err := ce.processExpression(enc, expr, statusArray, i)
			if errors.Is(err, errEvaluation) {
				ce.skippedIndices.set(i)
			}
			if err != nil {
				return err
			}
		}
		if haveReturn && multiReturn {
			if err := writeTokens(enc, jsontext.EndObject, jsontext.EndObject); err != nil {
				return err
			}
		}
		if haveLocals {
			if err := writeTokens(enc, jsontext.EndObject); err != nil {
				return err
			}
		}
	}

	// Capture expressions.
	if _, err := emitGroup(jsontext.String("captureExpressions"), ir.RootExpressionKindCaptureExpression); err != nil {
		return err
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
	if c.redaction.RedactType(valueType) {
		return writeRedacted(enc, valueType, tokenNotCapturedReasonRedactedType)
	}
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

// writeRedacted emits a captured-value object whose value is dropped for the
// given reason, e.g. {"type": "string", "notCapturedReason": "redactedIdent"}.
// The name (when there is one) is written by the caller beforehand.
func writeRedacted(enc *jsontext.Encoder, typeName string, reason jsontext.Token) error {
	return writeTokens(enc,
		jsontext.BeginObject,
		jsontext.String("type"),
		jsontext.String(typeName),
		tokenNotCapturedReason,
		reason,
		jsontext.EndObject,
	)
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
