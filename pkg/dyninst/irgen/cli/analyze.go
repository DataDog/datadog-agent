// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Cli for pc symbolication
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func analyze(path string, method string) error {
	binary, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open binary: %w", err)
	}
	defer binary.Close()

	obj, err := object.OpenElfFileWithDwarf(path)
	if err != nil {
		return fmt.Errorf("failed to open elf object: %w", err)
	}
	defer obj.Close()

	if method == "" {
		elf, err := safeelf.NewFile(binary)
		if err != nil {
			return fmt.Errorf("failed to parse elf: %w", err)
		}

		symbols, err := elf.Symbols()
		if err != nil {
			return fmt.Errorf("failed to get symbols: %w", err)
		}
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
		_, err = irgen.GenerateIR(1, obj, probes)
		if err != nil {
			return fmt.Errorf("failed to generate ir: %w", err)
		}
	} else {
		probes := []ir.ProbeDefinition{
			&rcjson.SnapshotProbe{
				LogProbeCommon: rcjson.LogProbeCommon{
					ProbeCommon: rcjson.ProbeCommon{
						ID:    "probe_" + method,
						Where: &rcjson.Where{MethodName: method},
					},
				},
			},
		}
		ir, err := irgen.GenerateIR(1, obj, probes)
		if err != nil {
			return fmt.Errorf("failed to generate ir: %w", err)
		}
		marshaled, err := irprinter.PrintYAML(ir)
		if err != nil {
			return fmt.Errorf("failed to print ir: %w", err)
		}
		fmt.Println(string(marshaled))
	}
	return nil
}

func main() {
	dyninsttest.SetupLogging()
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "Usage: analyze <binary> [method]")
		os.Exit(1)
	}
	method := ""
	if len(os.Args) == 3 {
		method = os.Args[2]
	}
	err := analyze(os.Args[1], method)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
