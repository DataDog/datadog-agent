// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gosym_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func BenchmarkParseGoSymbolTable(b *testing.B) {
	binPath, err := testprogs.GetBinary("simple", testprogs.Config{
		GOARCH:      "arm64",
		GOTOOLCHAIN: "go1.24.3",
	})
	require.NoError(b, err)
	mef, err := object.OpenMMappingElfFile(binPath)
	require.NoError(b, err)
	defer mef.Close()

	b.Run("ParseModuleData", func(b *testing.B) {
		for b.Loop() {
			_, err := object.ParseModuleData(mef)
			require.NoError(b, err)
		}
	})

	moduledata, err := object.ParseModuleData(mef)
	require.NoError(b, err)

	goDebugSections, err := moduledata.GoDebugSections(mef)
	require.NoError(b, err)
	defer func() { require.NoError(b, goDebugSections.Close()) }()

	b.Run("ParseGoSymbolTable", func(b *testing.B) {
		for b.Loop() {
			_, err := gosym.ParseGoSymbolTable(
				goDebugSections.PcLnTab.Data(),
				goDebugSections.GoFunc.Data(),
				moduledata.Text,
				moduledata.EText,
				moduledata.MinPC,
				moduledata.MaxPC,
			)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	symtab, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data(),
		goDebugSections.GoFunc.Data(),
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
	)
	require.NoError(b, err)

	b.Run("ListFunctions", func(b *testing.B) {
		for b.Loop() {
			symtab.CollectFunctions()
		}
	})

	var pcs []uint64
	for f := range symtab.Functions() {
		pcs = append(pcs, (f.Entry+f.End)/2)
	}

	b.Run("LocatePC", func(b *testing.B) {
		for b.Loop() {
			i := rand.Intn(len(pcs))
			symtab.LocatePC(pcs[i])
		}
	})
}
