// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyMaskSequence(t *testing.T) {
	rule := &ProcessingRule{
		Type:               MaskSequences,
		Name:               "mask_token",
		Pattern:            `token=[^ ]+`,
		ReplacePlaceholder: "token=[MASKED]",
	}
	require.NoError(t, CompileProcessingRules([]*ProcessingRule{rule}))

	masked, matched := ApplyMaskSequence([]byte("message token=secret"), rule)
	assert.True(t, matched)
	assert.Equal(t, []byte("message token=[MASKED]"), masked)

	unmatched, matched := ApplyMaskSequence([]byte("message without credentials"), rule)
	assert.False(t, matched)
	assert.Equal(t, []byte("message without credentials"), unmatched)
}

func TestApplyMaskSequenceIgnoresOtherRuleTypes(t *testing.T) {
	content := []byte("secret")
	masked, matched := ApplyMaskSequence(content, &ProcessingRule{Type: ExcludeAtMatch})
	assert.False(t, matched)
	assert.Equal(t, content, masked)
}
