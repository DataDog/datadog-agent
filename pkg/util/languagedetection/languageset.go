// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagedetection

import (
	"sort"
	"strings"
)

// LanguageSetInterface is an interface defining the behaviour of a language set
type LanguageSetInterface interface {
	Add(languageName string)
	Parse(languages string)
	String() string
}

// LanguageSet is a set of languages
type LanguageSet struct {
	languages map[string]struct{}
}

// NewLanguageSet initializes and returns a new LanguageSet object
func NewLanguageSet() *LanguageSet {
	return &LanguageSet{
		languages: make(map[string]struct{}),
	}
}

// Add adds a new language to the language set
func (langSet *LanguageSet) Add(languageName string) {
	if langSet.languages == nil {
		langSet.languages = make(map[string]struct{})
	}

	langSet.languages[languageName] = struct{}{}
}

// Parse parses a comma-separated languages string and adds the languages to the language set
func (langSet *LanguageSet) Parse(languages string) {
	if langSet.languages == nil {
		langSet.languages = map[string]struct{}{}
	}

	for _, languageName := range strings.Split(languages, ",") {
		if languageName != "" {
			langSet.Add(languageName)
		}
	}
}

// String prints the languages of the language set in sorted order in a comma-separated string format
func (langSet *LanguageSet) String() string {
	langNames := make([]string, 0, len(langSet.languages))
	for name := range langSet.languages {
		langNames = append(langNames, name)
	}
	sort.Strings(langNames)
	return strings.Join(langNames, ",")
}
