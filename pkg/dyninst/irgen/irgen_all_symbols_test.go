// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestIRGenAllProbes(t *testing.T) {
	programs := testprogs.MustGetPrograms(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, pkg := range programs {
		switch pkg {
		case "simple", "sample":
		default:
			// TODO: The generation for programs that link dd-trace-go is
			// very slow due to accidentally quadratic behavior when processing
			// the line programs. We should fix this, but for now we skip these
			// programs.
			t.Logf("skipping %s", pkg)
			continue
		}
		t.Run(pkg, func(t *testing.T) {
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					bin := testprogs.MustGetBinary(t, pkg, cfg)
					testAllProbes(t, bin)
				})
			}
		})
	}
}

func testAllProbes(t *testing.T, sampleServicePath string) {
	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	symbols, err := binary.Symbols()
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()
	var probes []ir.ProbeDefinition
	for i, s := range symbols {
		// These automatically generated symbols cause problems.
		if strings.HasPrefix(s.Name, "type:.") {
			continue
		}
		if strings.HasPrefix(s.Name, "runtime.vdso") {
			continue
		}

		// Speed things up by skipping some symbols.
		probes = append(probes, &rcjson.SnapshotProbe{
			LogProbeCommon: rcjson.LogProbeCommon{
				ProbeCommon: rcjson.ProbeCommon{
					ID:    fmt.Sprintf("probe_%d", i),
					Where: &rcjson.Where{MethodName: s.Name},
				},
			},
		})
	}

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)
	v, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.NotNil(t, v)
	// TODO: Validate more properties of the IR.
}
