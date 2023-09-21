// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainersLanguageParse(t *testing.T) {
	containerslanguages := NewContainersLanguages()

	mockContainerName := "nginx"
	mockLanguages := "java,cpp,python"

	// Test if it works with empty languageset
	containerslanguages.Parse(mockContainerName, mockLanguages)

	expectedLanguages := map[string]struct{}{
		"java":   {},
		"cpp":    {},
		"python": {},
	}

	actualLanguages := containerslanguages.Languages[mockContainerName].languages
	assert.Equal(t, expectedLanguages, actualLanguages)

	// Test if it works with prefilled languageset
	mockAdditionalLanguages := "golang,csharp"
	containerslanguages.Parse(mockContainerName, mockAdditionalLanguages)
	expectedLanguages = map[string]struct{}{
		"java":   {},
		"cpp":    {},
		"python": {},
		"golang": {},
		"csharp": {},
	}
	actualLanguages = containerslanguages.Languages[mockContainerName].languages
	assert.Equal(t, expectedLanguages, actualLanguages)
}

func TestContainersLanguagesAdd(t *testing.T) {
	containerslanguages := NewContainersLanguages()

	mockContainerName := "nginx"
	mockLanguage := "java"

	// Test if it works with empty languageset
	containerslanguages.Add(mockContainerName, mockLanguage)

	expectedLanguages := map[string]struct{}{
		"java": {},
	}

	actualLanguages := containerslanguages.Languages[mockContainerName].languages
	assert.Equal(t, expectedLanguages, actualLanguages)

	// Test if it works with prefilled languageset
	mockAdditionalLanguage := "golang"
	containerslanguages.Add(mockContainerName, mockAdditionalLanguage)
	expectedLanguages = map[string]struct{}{
		"java":   {},
		"golang": {},
	}
	actualLanguages = containerslanguages.Languages[mockContainerName].languages
	assert.Equal(t, expectedLanguages, actualLanguages)
}

func TestTotalLanguages(t *testing.T) {
	containerslanguages := &ContainersLanguages{
		Languages: map[string]*LanguageSet{
			"nginx": NewLanguageSet(),
			"wordpress": {
				languages: map[string]struct{}{
					"php":        {},
					"javascript": {},
				},
			},
			"server": {
				languages: map[string]struct{}{
					"python":     {},
					"cpp":        {},
					"javascript": {},
				},
			},
		},
	}

	expectedTotalLanguages := 5
	actualTotalLanguages := containerslanguages.TotalLanguages()

	assert.Equal(t, expectedTotalLanguages, actualTotalLanguages)
}

func TestParseAnnotations(t *testing.T) {
	mockAnnotations := map[string]string{
		"apm.datadoghq.com/cont-1.languages": "java,cpp,python",
		"apm.datadoghq.com/cont-2.languages": "javascript,cpp,golang",
		"annotationkey1":                     "annotationvalue1",
		"annotationkey2":                     "annotationvalue2",
	}

	containerslanguages := NewContainersLanguages()

	containerslanguages.ParseAnnotations(mockAnnotations)

	// Test that two containers languagesets were added
	assert.Equal(t, 2, len(containerslanguages.Languages))

	expectedlanguages1 := map[string]struct{}{
		"java":   {},
		"cpp":    {},
		"python": {},
	}

	expectedlanguages2 := map[string]struct{}{
		"javascript": {},
		"cpp":        {},
		"golang":     {},
	}

	assert.Equal(t, expectedlanguages1, containerslanguages.Languages["cont-1"].languages)
	assert.Equal(t, expectedlanguages2, containerslanguages.Languages["cont-2"].languages)
}

func TestToAnnotations(t *testing.T) {
	containerslanguages := &ContainersLanguages{
		Languages: map[string]*LanguageSet{
			"nginx": NewLanguageSet(),
			"wordpress": {
				languages: map[string]struct{}{
					"php":        {},
					"javascript": {},
				},
			},
			"server": {
				languages: map[string]struct{}{
					"python":     {},
					"cpp":        {},
					"javascript": {},
				},
			},
		},
	}

	actualAnnotations := containerslanguages.ToAnnotations()
	expectedAnnotations := map[string]string{
		"apm.datadoghq.com/wordpress.languages": "javascript,php",
		"apm.datadoghq.com/server.languages":    "cpp,javascript,python",
	}

	assert.Equal(t, expectedAnnotations, actualAnnotations)
}
