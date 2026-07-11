// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// TestContainsAnyOpcodeEquivalence asserts that `contains(coll, key)` and
// `any(coll, {@it == key})` over slices and arrays produce the same IR
// Operations sequence in all three positions where the call can appear:
// when: clause (Condition), template-segment expression, and capture-
// expression entry. This is the load-bearing invariant of the desugaring
// in emitContainsLeaf and resolveContainsExpression.
func TestContainsAnyOpcodeEquivalence(t *testing.T) {
	const testProg = "simple"
	condPairs := []struct{ anyID, containsID string }{
		{"anyIntSlice_eq42", "containsIntSlice_eq42"},
		{"anyIntArray_eq42", "containsIntArray_eq42"},
		{"anyStringSlice_match", "containsStringSlice_match"},
		{"anyStringArray_match", "containsStringArray_match"},
	}
	templatePairs := []struct{ anyID, containsID string }{
		{"templateAnyIntSlice_eq42", "templateContainsIntSlice_42"},
	}
	capturePairs := []struct{ anyID, containsID string }{
		{"captureAnyIntSlice_eq42", "captureContainsIntSlice_42"},
	}
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			bin := testprogs.MustGetBinary(t, testProg, cfg)
			obj, err := object.OpenElfFileWithDwarf(bin)
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()

			probes := testprogs.MustGetProbeDefinitions(t, testProg)
			p, err := irgen.GenerateIR(1, obj, probes)
			require.NoError(t, err)

			byID := make(map[string]*ir.Probe, len(p.Probes))
			for _, pr := range p.Probes {
				byID[pr.GetID()] = pr
			}

			entryEvent := func(t *testing.T, id string) *ir.Event {
				t.Helper()
				pr, ok := byID[id]
				require.Truef(t, ok, "probe %q missing from generated IR", id)
				require.Lenf(t, pr.Instances, 1, "probe %q: expected exactly one instance", id)
				for _, ev := range pr.Instances[0].Events {
					if ev.Kind == ir.EventKindEntry {
						return ev
					}
				}
				t.Fatalf("probe %q: no Entry event", id)
				return nil
			}
			entryCondOps := func(t *testing.T, id string) []ir.ExpressionOp {
				t.Helper()
				ev := entryEvent(t, id)
				require.NotNilf(t, ev.Condition, "probe %q: entry event has no Condition", id)
				return ev.Condition.Operations
			}
			entryRootOps := func(t *testing.T, id string, kind ir.RootExpressionKind) []ir.ExpressionOp {
				t.Helper()
				ev := entryEvent(t, id)
				require.NotNilf(t, ev.Type, "probe %q: entry event has no Type", id)
				for _, re := range ev.Type.Expressions {
					if re.Kind == kind {
						return re.Expression.Operations
					}
				}
				t.Fatalf("probe %q: no %s expression on entry event", id, kind.String())
				return nil
			}

			t.Run("condition", func(t *testing.T) {
				for _, pair := range condPairs {
					t.Run(pair.containsID, func(t *testing.T) {
						anyOps := entryCondOps(t, pair.anyID)
						containsOps := entryCondOps(t, pair.containsID)
						require.Equalf(t, anyOps, containsOps,
							"condition opcodes for %s and %s diverged",
							pair.anyID, pair.containsID)
					})
				}
			})
			t.Run("template", func(t *testing.T) {
				for _, pair := range templatePairs {
					t.Run(pair.containsID, func(t *testing.T) {
						anyOps := entryRootOps(t, pair.anyID, ir.RootExpressionKindTemplateSegment)
						containsOps := entryRootOps(t, pair.containsID, ir.RootExpressionKindTemplateSegment)
						require.Equalf(t, anyOps, containsOps,
							"template-segment opcodes for %s and %s diverged",
							pair.anyID, pair.containsID)
					})
				}
			})
			t.Run("capture", func(t *testing.T) {
				for _, pair := range capturePairs {
					t.Run(pair.containsID, func(t *testing.T) {
						anyOps := entryRootOps(t, pair.anyID, ir.RootExpressionKindCaptureExpression)
						containsOps := entryRootOps(t, pair.containsID, ir.RootExpressionKindCaptureExpression)
						require.Equalf(t, anyOps, containsOps,
							"capture-expression opcodes for %s and %s diverged",
							pair.anyID, pair.containsID)
					})
				}
			})
		})
	}
}
