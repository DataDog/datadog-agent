// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	"os"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
)

func FuzzParse(f *testing.F) {
	// Seed from golden file inputs.
	raw, err := os.ReadFile("testdata/symbols.yaml")
	require.NoError(f, err)
	var inputs []testCaseInput
	require.NoError(f, yaml.Unmarshal(raw, &inputs))
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
