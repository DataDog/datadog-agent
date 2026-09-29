// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenizerContract(t *testing.T) {
	assert.Equal(t, Contract{Profile: CompactProfile, Version: ProfileVersion, MaxInputBytes: 2048}, NewTokenizer(2048).Contract())
	assert.Equal(t, Contract{Profile: CompactProfile, Version: ProfileVersion}, NewTokenizer(0).Contract())
}

func TestContractCompatible(t *testing.T) {
	base := Contract{Profile: CompactProfile, Version: ProfileVersion}
	with := func(maxInputBytes, maxTokens int) Contract {
		c := base
		c.MaxInputBytes = maxInputBytes
		c.MaxTokens = maxTokens
		return c
	}

	tests := []struct {
		name string
		have Contract
		need Contract
		want bool
	}{
		{"identical", with(2048, 0), with(2048, 0), true},
		{"wider window serves narrower", with(2048, 0), with(60, 0), true},
		{"narrower window cannot serve wider", with(60, 0), with(2048, 0), false},
		{"unlimited window serves any", with(0, 0), with(12500, 0), true},
		{"limited window cannot serve unlimited", with(2048, 0), with(0, 0), false},
		{"negative means unlimited", with(-1, 0), with(12500, 0), true},
		{"token cap: no cap serves a cap", with(2048, 0), with(60, 250), true},
		{"token cap: smaller cap cannot serve larger", with(2048, 100), with(60, 250), false},
		{"token cap: larger cap serves smaller", with(2048, 250), with(60, 100), true},
		{"token cap: a cap cannot serve no cap", with(2048, 250), with(60, 0), false},
		{"different version", with(0, 0), Contract{Profile: CompactProfile, Version: ProfileVersion + 1}, false},
		{"different profile", with(0, 0), Contract{Profile: "other", Version: ProfileVersion}, false},
		{"zero values are compatible", Contract{}, Contract{}, true},
		{"zero value does not match the compact profile", Contract{}, base, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.have.Compatible(tt.need))
		})
	}
}

func TestContractCompatibleWithTokenizer(t *testing.T) {
	// The labeler (60 B) reads its window off the sampler's wider pass.
	sampler := NewTokenizer(2048).Contract()
	labeler := NewTokenizer(60).Contract()

	assert.True(t, sampler.Compatible(labeler))
	assert.False(t, labeler.Compatible(sampler))
}
