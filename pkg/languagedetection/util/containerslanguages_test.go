// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTotalLanguages(t *testing.T) {
	containerslanguages := &ContainersLanguages{
		"nginx": {},
		"wordpress": {
			"php":        {},
			"javascript": {},
		},
		"server": {
			"python":     {},
			"cpp":        {},
			"javascript": {},
		},
	}

	expectedTotalLanguages := 5
	actualTotalLanguages := containerslanguages.TotalLanguages()

	assert.Equal(t, expectedTotalLanguages, actualTotalLanguages)
}

func TestParseAnnotations(t *testing.T) {
	mockAnnotations := map[string]string{
		"apm.datadoghq.com/cont-1.languages":      "java,cpp,python",
		"apm.datadoghq.com/cont-2.languages":      "javascript,cpp,golang",
		"apm.datadoghq.com/init.cont-3.languages": "python,java",
		"annotationkey1":                          "annotationvalue1",
		"annotationkey2":                          "annotationvalue2",
	}

	containerslanguages := NewContainersLanguages()

	containerslanguages.ParseAnnotations(mockAnnotations)

	// Test that three containers languagesets were added
	assert.Equal(t, 3, len(containerslanguages))

	expectedlanguages1 := LanguageSet{
		"java":   {},
		"cpp":    {},
		"python": {},
	}

	expectedlanguages2 := LanguageSet{
		"javascript": {},
		"cpp":        {},
		"golang":     {},
	}

	expectedlanguages3 := LanguageSet{
		"python": {},
		"java":   {},
	}

	assert.Equal(t, expectedlanguages1, containerslanguages["cont-1"])
	assert.Equal(t, expectedlanguages2, containerslanguages["cont-2"])
	assert.Equal(t, expectedlanguages3, containerslanguages["init.cont-3"])
}

func TestToAnnotations(t *testing.T) {
	containerslanguages := &ContainersLanguages{

		"nginx": NewLanguageSet(),
		"wordpress": {

			"php":        {},
			"javascript": {},
		},
		"server": {

			"python":     {},
			"cpp":        {},
			"javascript": {},
		},
		"init.launcher": {

			"bash": {},
			"cpp":  {},
		},
	}

	actualAnnotations := containerslanguages.ToAnnotations()
	expectedAnnotations := map[string]string{
		"apm.datadoghq.com/wordpress.languages":     "javascript,php",
		"apm.datadoghq.com/server.languages":        "cpp,javascript,python",
		"apm.datadoghq.com/init.launcher.languages": "bash,cpp",
	}

	assert.Equal(t, expectedAnnotations, actualAnnotations)
}
