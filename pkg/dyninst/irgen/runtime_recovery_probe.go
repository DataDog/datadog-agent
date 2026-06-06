// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"encoding/json"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// runtimeRecoveryProbeID is the synthetic ID for the internal probe
// attached to runtime.recovery. The double-underscore prefix mirrors
// reserved-name conventions and avoids any plausible collision with
// user-provided probe IDs (which are derived from rcjson payload UUIDs).
// The canonical declaration lives in the ir package so consumers
// outside irgen (e.g. the actuator's circuit breaker) can recognise the
// probe by ID without an irgen dependency.
const runtimeRecoveryProbeID = ir.RuntimeRecoveryProbeID

// runtimeRecoveryFunc is the target function in the Go runtime.
//
// runtime.recovery is called via mcall(recovery) from runtime.gopanic's
// deferred-call loop after gorecover has set p.recovered=true. At entry
// gp._panic still points at the recovering panic and its startSP/sp
// fields bound the stack region that's about to be unwound without
// normal returns. See runtime/panic.go:1138.
const runtimeRecoveryFunc = "runtime.recovery"

// runtimeRecoveryProbeDef is an internal ProbeDefinition synthesised
// by irgen when the program has at least one function-targeted user
// probe. It attaches a uprobe to runtime.recovery and runs through
// the standard stack-machine pipeline with a single @exception capture
// expression (built by synthesizeRecoveryProbeEventRoot) bookended by
// PanicUnwindPrepareOp / PanicUnwindEvictSlotsOp. The ops validate
// the recovered panic, emit one synthetic event carrying the panic
// value, and zero every in_progress_calls slot in the unwound stack
// region — preventing both the BPF call-depths slot and the userspace
// bufferedEvent from leaking. The probe is never sourced from rcjson;
// it has no template, no condition, and no throttle.
type runtimeRecoveryProbeDef struct{}

var _ ir.ProbeDefinition = runtimeRecoveryProbeDef{}

// GetID implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetID() string { return runtimeRecoveryProbeID }

// GetVersion implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetVersion() int { return 0 }

// GetKind implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetKind() ir.ProbeKind {
	return ir.ProbeKindRuntimeRecovery
}

// GetWhere implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetWhere() ir.Where {
	return runtimeRecoveryWhere{}
}

// GetTags implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetTags() []string { return nil }

// GetTemplate implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetTemplate() ir.TemplateDefinition { return nil }

// GetWhen implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetWhen() json.RawMessage { return nil }

// GetWhenDSL implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetWhenDSL() string { return "" }

// GetCaptureExpressions implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetCaptureExpressions() []ir.CaptureExpressionDefinition {
	return nil
}

// GetCaptureConfig implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetCaptureConfig() ir.CaptureConfig {
	return runtimeRecoveryCaptureConfig{}
}

// GetThrottleConfig implements ir.ProbeDefinition.
func (runtimeRecoveryProbeDef) GetThrottleConfig() ir.ThrottleConfig {
	return runtimeRecoveryThrottleConfig{}
}

// runtimeRecoveryWhere is the FunctionWhere pointing at runtime.recovery.
type runtimeRecoveryWhere struct{}

var _ ir.FunctionWhere = runtimeRecoveryWhere{}

// Where implements ir.Where (marker method).
func (runtimeRecoveryWhere) Where() {}

// Location implements ir.FunctionWhere.
func (runtimeRecoveryWhere) Location() string { return runtimeRecoveryFunc }

// runtimeRecoveryCaptureConfig satisfies ir.CaptureConfig with limits
// sized for typical panic values. The recovery probe captures one
// empty interface (the panic value) and chases its pointee through
// the standard pointer-chasing pipeline, so we want enough chase
// budget to follow e.g. *errors.errorString through to the string
// body.
type runtimeRecoveryCaptureConfig struct{}

var _ ir.CaptureConfig = runtimeRecoveryCaptureConfig{}

func (runtimeRecoveryCaptureConfig) GetMaxReferenceDepth() uint32 { return 4 }
func (runtimeRecoveryCaptureConfig) GetMaxCollectionSize() uint32 { return 64 }
func (runtimeRecoveryCaptureConfig) GetMaxFieldCount() uint32     { return 64 }
func (runtimeRecoveryCaptureConfig) GetMaxLength() uint32         { return 512 }

// runtimeRecoveryThrottleConfig satisfies ir.ThrottleConfig with no
// throttling — the BPF dispatcher routes recovery firings before the
// throttle check.
type runtimeRecoveryThrottleConfig struct{}

var _ ir.ThrottleConfig = runtimeRecoveryThrottleConfig{}

func (runtimeRecoveryThrottleConfig) GetThrottlePeriodMs() uint32 { return 0 }
func (runtimeRecoveryThrottleConfig) GetThrottleBudget() int64    { return 0 }

// maybeAddRuntimeRecoveryProbe appends the synthetic recovery probe
// definition to defs when at least one input probe targets a function
// (FunctionWhere). Line probes alone don't trigger the recovery probe:
// line probes don't establish in_progress_calls state, so they cannot
// leak across a panic-recover boundary.
func maybeAddRuntimeRecoveryProbe(defs []ir.ProbeDefinition) []ir.ProbeDefinition {
	hasFunctionProbe := false
	for _, d := range defs {
		if _, ok := d.GetWhere().(ir.FunctionWhere); ok {
			hasFunctionProbe = true
			break
		}
	}
	if !hasFunctionProbe {
		return defs
	}
	return append(defs, runtimeRecoveryProbeDef{})
}

// synthesizeRecoveryProbes builds the recovery probe's EventRootType
// (via synthesizeRecoveryProbeEventRoot) for every recovery probe in
// the input. Probes whose synthesis fails (binary lacks runtime._panic
// or runtime.eface; recovery's subprogram has no out-of-line range) are
// removed from the returned slice — a warning is logged so operators
// can see panic-unwind handling is disabled for that binary.
func synthesizeRecoveryProbes(
	probes []*ir.Probe,
	commonTypes ir.CommonTypes,
	tc *typeCatalog,
) []*ir.Probe {
	return slices.DeleteFunc(probes, func(probe *ir.Probe) bool {
		if probe.GetKind() != ir.ProbeKindRuntimeRecovery {
			return false
		}
		if !synthesizeRecoveryProbeEventRoot(probe, commonTypes, tc) {
			log.Warnf(
				"dyninst: runtime.recovery probe could not be " +
					"synthesised (missing runtime types); panic-recover " +
					"leaks will not be cleaned up for this binary",
			)
			return true
		}
		return false
	})
}

// synthesizeRecoveryProbeEventRoot fills in the recovery probe's
// Event.Type (EventRootType) with a single @exception capture expression
// whose location reads gp.{_panic}.{arg} via a Register + Deref +
// Deref chain. The expression's value type is runtime.eface (an empty
// interface), which gets the standard GoEmptyInterfaceType handling
// (PROCESS_GO_EMPTY_INTERFACE + CHASE_POINTERS) at compile time and
// renders as a typed Go value via encodeInterface at decode time.
//
// The expression Operations are bookended with PanicUnwindPrepareOp
// (validates the panic and computes panic_lo_depth/panic_hi_depth
// into the event header) and PanicUnwindEvictSlotsOp (zeroes the
// in_progress_calls slots in the unwound region).
//
// Returns false (with a warning) if the binary's catalog lacks
// runtime.eface; the recovery probe stays disabled in that case.
func synthesizeRecoveryProbeEventRoot(
	probe *ir.Probe,
	commonTypes ir.CommonTypes,
	tc *typeCatalog,
) bool {
	if commonTypes.Panic == nil {
		return false
	}
	// gp has type *runtime.g; we read it from DWARF register 0.
	// runtime.recovery's signature is `func recovery(gp *g)`, so the
	// first argument is in DWARF register 0 (AX on amd64, X0 on
	// arm64) per Go's ABIInternal.
	panicField, ok := commonTypes.G.FieldByName("_panic")
	if !ok {
		return false
	}
	argField, ok := commonTypes.Panic.FieldByName("arg")
	if !ok {
		return false
	}
	// argField.Type should be *ir.GoEmptyInterfaceType (an `any`).
	efaceType, ok := argField.Type.(*ir.GoEmptyInterfaceType)
	if !ok {
		return false
	}
	// The BPF SM_OP_PANIC_UNWIND_PREPARE handler reads startSP and sp to
	// compute the unwound (lo, hi] depth range; without both, every
	// recovery firing would read offset 0 from runtime._panic for each
	// and silently bail out only because the values happen to compare
	// equal. Drop the probe entirely rather than attach a no-op that
	// relies on that coincidence. The recovered / goexit filters are
	// gated by OFFSET != 0 in BPF so they may legitimately be absent.
	for _, name := range []string{"startSP", "sp"} {
		if _, ok := commonTypes.Panic.FieldByName(name); !ok {
			return false
		}
	}

	const ptrSize = 8
	const interfaceSize = 16

	// Synthetic variable for gp. The IR location lists a single piece
	// referencing DWARF register 0 (ABIInternal arg 0). The variable is
	// appended to the recovery subprogram's Variables so downstream
	// consumers (irprinter, decode) can find it via the standard
	// subprogram lookup. The name is prefixed with '@' (matching
	// @duration / @trace_context / @exception) so it can never collide
	// with a DWARF-derived variable name — runtime.recovery's real
	// parameter is also named gp, and a plain "gp" would produce a
	// duplicate entry in Subprogram.Variables.
	subprog := probe.Instances[0].Subprogram
	gpVar := &ir.Variable{
		Name: "@gp",
		Type: panicField.Type, // *runtime._panic — a pointer-sized value
		Locations: []ir.Location{{
			Range:  subprog.OutOfLinePCRanges[0],
			Pieces: []ir.Piece{{Size: ptrSize, Op: ir.Register{RegNo: 0}}},
		}},
		Role:      ir.VariableRoleParameter,
		DictIndex: -1,
	}
	subprog.Variables = append(subprog.Variables, gpVar)

	expr := ir.Expression{
		Type: efaceType,
		Operations: []ir.ExpressionOp{
			// 1. Validate the recovered panic and stash unwound-region
			//    depth bounds in the event header. On failure this op
			//    sets condition_failed and aborts the stack machine
			//    (so probe_run skips submitting the event).
			&ir.PanicUnwindPrepareOp{},
			// 2. Read gp from register 0 (8 bytes — pointer).
			&ir.LocationOp{
				Variable: gpVar,
				Offset:   0,
				ByteSize: ptrSize,
			},
			// 3. Deref gp -> _panic pointer (8 bytes).
			&ir.DereferenceOp{
				Bias:     panicField.Offset,
				ByteSize: ptrSize,
			},
			// 4. Deref _panic -> arg (the 16-byte interface header).
			&ir.DereferenceOp{
				Bias:     argField.Offset,
				ByteSize: interfaceSize,
			},
			// 5. Clear the matching in_progress_calls slots. Reads
			//    goid + (lo, hi) from the event header populated by
			//    PanicUnwindPrepareOp.
			&ir.PanicUnwindEvictSlotsOp{},
		},
	}

	rootExpr := &ir.RootExpression{
		Name:       "@exception",
		Offset:     0, // set below after ExprStatusArraySize is computed
		Kind:       ir.RootExpressionKindCaptureExpression,
		Expression: expr,
		DictIndex:  -1,
	}

	// One expression -> 1 status entry. Status array uses
	// ExprStatusBits per entry, rounded up to bytes.
	exprStatusArraySize := uint32((ir.ExprStatusBits*1 + 7) / 8)
	rootExpr.Offset = exprStatusArraySize
	byteSize := uint64(exprStatusArraySize) + uint64(interfaceSize)

	rootType := &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     "Probe[" + subprog.Name + "]",
			ByteSize: uint32(byteSize),
		},
		EventKind:           ir.EventKindEntry,
		ExprStatusArraySize: exprStatusArraySize,
		Expressions:         []*ir.RootExpression{rootExpr},
	}
	tc.typesByID[rootType.ID] = rootType

	probe.Instances[0].Events[0].Type = rootType
	return true
}
