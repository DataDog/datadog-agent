// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOTTLStructuralValidation(t *testing.T) {
	for _, kind := range []string{IncludeAtMatchOTTL, ExcludeAtMatchOTTL, MaskOTTLTransform} {
		t.Run(kind, func(t *testing.T) {
			rules := []*ProcessingRule{{Type: kind, Name: "test", Pattern: "invalid OTTL syntax"}}
			// Shared configuration checks structure without linking an OTTL parser.
			require.NoError(t, ValidateProcessingRules(rules))
			require.NoError(t, CompileProcessingRules(rules))
			require.Nil(t, rules[0].OTTL)
			rules[0].Pattern = ""
			require.ErrorContains(t, ValidateProcessingRules(rules), "no pattern provided")
		})
	}
}
