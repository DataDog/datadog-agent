// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagedetection

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	mockLanguageSet := NewLanguageSet()

	mockLanguageSet.Add("cpp")
	mockLanguageSet.Add("java")

	expectedLanguages := map[string]struct{}{
		"cpp":  {},
		"java": {},
	}

	actualLanguages := mockLanguageSet.languages

	assert.Equal(t, expectedLanguages, actualLanguages)
}

func TestParse(t *testing.T) {
	languageSet := NewLanguageSet()
	mockLanguages := "cpp,java,ruby"
	languageSet.Parse(mockLanguages)

	expectedLanguages := map[string]struct{}{
		"cpp":  {},
		"java": {},
		"ruby": {},
	}

	actualLanguages := languageSet.languages

	assert.Equal(t, expectedLanguages, actualLanguages)
}

func TestString(t *testing.T) {
	languageSet := &LanguageSet{
		languages: map[string]struct{}{
			"cpp":  {},
			"ruby": {},
			"java": {},
		},
	}

	expectedOutput := "cpp,java,ruby"
	actualOutput := fmt.Sprint(languageSet)

	assert.Equal(t, expectedOutput, actualOutput)
}
