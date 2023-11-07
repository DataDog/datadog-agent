// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	mockLanguageSet := NewLanguageSet()

	mockLanguageSet.Add("cpp")
	mockLanguageSet.Add("java")

	expectedLanguages := LanguageSet{
		"cpp":  {},
		"java": {},
	}

	assert.Equal(t, expectedLanguages, mockLanguageSet)
}

func TestParse(t *testing.T) {
	languageSet := NewLanguageSet()
	mockLanguages := "cpp,java,ruby"
	languageSet.Parse(mockLanguages)

	expectedLanguages := LanguageSet{
		"cpp":  {},
		"java": {},
		"ruby": {},
	}

	assert.Equal(t, expectedLanguages, languageSet)
}

func TestMerge(t *testing.T) {
	languageSet := LanguageSet{
		"cpp":  {},
		"java": {},
		"ruby": {},
	}

	languageSetToMerge := LanguageSet{
		"cpp":    {},
		"python": {},
		"ruby":   {},
	}

	expectedLanguageSet := LanguageSet{
		"cpp":    {},
		"java":   {},
		"ruby":   {},
		"python": {},
	}

	languageSet.Merge(languageSetToMerge)
	assert.Equal(t, expectedLanguageSet, languageSet)
}

func TestString(t *testing.T) {
	languageSet := &LanguageSet{
		"cpp":  {},
		"ruby": {},
		"java": {},
	}

	expectedOutput := "cpp,java,ruby"
	actualOutput := fmt.Sprint(languageSet)

	assert.Equal(t, expectedOutput, actualOutput)
}
