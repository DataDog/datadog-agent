// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gosym

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func BenchmarkParseGoSymbolTable(b *testing.B) {
	elfFile, err := safeelf.Open("/home/piob/src/github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/binaries/arch=amd64,toolchain=go1.24.3/simple")
	if err != nil {
		b.Fatal(err)
	}
	defer elfFile.Close()

	b.Run("ParseModuleData", func(b *testing.B) {
		for b.Loop() {
			_, err := object.ParseModuleData(elfFile)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	moduledata, err := object.ParseModuleData(elfFile)
	if err != nil {
		b.Fatal(err)
	}

	goVersion, err := object.ParseGoVersion(elfFile)
	if err != nil {
		b.Fatal(err)
	}

	goDebugSections, err := moduledata.GoDebugSections(elfFile)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("ParseGoSymbolTable", func(b *testing.B) {
		for b.Loop() {
			_, err := ParseGoSymbolTable(
				goDebugSections.PcLnTab,
				goDebugSections.GoFunc,
				moduledata.Text,
				moduledata.EText,
				moduledata.MinPC,
				moduledata.MaxPC,
				goVersion,
			)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	symtab, err := ParseGoSymbolTable(
		goDebugSections.PcLnTab,
		goDebugSections.GoFunc,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("ListFunctions", func(b *testing.B) {
		for b.Loop() {
			symtab.Functions()
		}
	})

	var pcs []uint64
	for _, f := range symtab.Functions() {
		pcs = append(pcs, (f.Entry+f.End)/2)
	}

	b.Run("LocatePC", func(b *testing.B) {
		for b.Loop() {
			i := rand.Intn(len(pcs))
			symtab.LocatePC(pcs[i])
		}
	})
}
