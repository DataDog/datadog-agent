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
// Operations sequence on the Entry event's Condition. This is the load-
// bearing invariant of the desugaring in emitContainsLeaf.
func TestContainsAnyOpcodeEquivalence(t *testing.T) {
	const testProg = "simple"
	pairs := []struct{ anyID, containsID string }{
		{"anyIntSlice_eq42", "containsIntSlice_eq42"},
		{"anyIntArray_eq42", "containsIntArray_eq42"},
		{"anyStringSlice_match", "containsStringSlice_match"},
		{"anyStringArray_match", "containsStringArray_match"},
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

			entryCondOps := func(t *testing.T, id string) []ir.ExpressionOp {
				t.Helper()
				pr, ok := byID[id]
				require.Truef(t, ok, "probe %q missing from generated IR", id)
				require.Lenf(t, pr.Instances, 1, "probe %q: expected exactly one instance", id)
				inst := pr.Instances[0]
				for _, ev := range inst.Events {
					if ev.Kind == ir.EventKindEntry {
						require.NotNilf(t, ev.Condition, "probe %q: entry event has no Condition", id)
						return ev.Condition.Operations
					}
				}
				t.Fatalf("probe %q: no Entry event", id)
				return nil
			}

			for _, pair := range pairs {
				t.Run(pair.containsID, func(t *testing.T) {
					anyOps := entryCondOps(t, pair.anyID)
					containsOps := entryCondOps(t, pair.containsID)
					require.Equalf(t, anyOps, containsOps,
						"opcodes for %s and %s diverged", pair.anyID, pair.containsID)
				})
			}
		})
	}
}
