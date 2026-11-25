// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// The fuzzing infrastructure uploads only the compiled test binary, so keep
// the fuzz seeds in the binary instead of depending on the worker's checkout.
//
//go:embed testdata/symbols.yaml
var symbolsFuzzFixture []byte

func FuzzParse(f *testing.F) {
	// Seed from golden file inputs.
	var inputs []testCaseInput
	require.NoError(f, yaml.Unmarshal(symbolsFuzzFixture, &inputs))
	for _, in := range inputs {
		f.Add(in.Input.Symbol, uint8(parseSource(in.Input.Source)))
	}

	f.Fuzz(func(_ *testing.T, symbol string, sourceRaw uint8) {
		source := SymbolSource(sourceRaw % 5) // keep in valid range

		// Parse must not panic.
		s := Parse(symbol, source)

		// Accessing all fields must not panic.
		_ = s.Raw()
		_ = s.Package()
		_ = s.Local()
		_ = s.Class()
		_ = s.IsGeneric()
		_ = s.IsCompilerGenerated()
		_ = s.IsClosure()
		_ = s.HasPointerReceiver()
		_ = s.IsAmbiguous()

		// Full interpretation must not panic.
		interps := s.Interpretations()
		for i := range interps {
			_ = interps[i].QualifiedName()
			_ = interps[i].IsMethod()
			_ = interps[i].IsGeneric()
			_ = interps[i].HasInlinedCalls()
			_ = interps[i].BaseName()
			for j := range interps[i].InlinedCalls {
				_ = interps[i].InlinedCalls[j].QualifiedFunction()
				_ = interps[i].InlinedCalls[j].IsMethod()
			}
		}

		// ParseInto must not panic.
		var s2 Symbol
		ParseInto(&s2, symbol, source)

		// SplitPackage must not panic.
		SplitPackage(symbol, source)
	})
}
