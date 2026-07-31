// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorpus translates every expression of a corpus of real SECL rules,
// collected from the SECL evaluator tests, the SECL fuzzing seeds and the
// runtime security functional tests. Every one of them is accepted by the SECL
// parser, so every one of them has to translate.
//
// Each translation is also parsed back as CEL and printed again: the two
// languages do not agree on operator precedence, so this is what proves that
// the CEL source the translator prints regroups the expression the way SECL
// grouped it.
func TestCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/corpus.json")
	require.NoError(t, err)

	var corpus []string
	require.NoError(t, json.Unmarshal(raw, &corpus))
	require.NotEmpty(t, corpus)

	// Parsing does not resolve names, so the corpus needs no field declarations:
	// this checks the shape of the translation, not its resolution against a
	// model.
	env, err := NewEnv()
	require.NoError(t, err)

	for _, expr := range corpus {
		t.Run(expr, func(t *testing.T) {
			translated, err := Translate(expr)
			require.NoError(t, err)

			parsed, iss := env.Parse(translated)
			require.NoError(t, iss.Err(), "the translation is not valid CEL")

			reparsed, err := cel.AstToString(parsed)
			require.NoError(t, err)
			assert.Equal(t, translated, reparsed, "the translation does not survive a CEL round trip")
		})
	}
}
