// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagedetection

import (
	"sort"
	"strings"
)

// LanguageSet is a set of languages
type LanguageSet map[string]struct{}

// NewLanguageSet initializes and returns a new LanguageSet object
func NewLanguageSet() LanguageSet {
	return make(LanguageSet)
}

// Add adds a new language to the language set
func (langSet LanguageSet) Add(languageName string) {
	langSet[languageName] = struct{}{}
}

// Parse parses a comma-separated languages string and adds the languages to the language set
func (langSet LanguageSet) Parse(languages string) {
	for _, languageName := range strings.Split(languages, ",") {
		if languageName != "" {
			langSet.Add(languageName)
		}
	}
}

// Merge merges a set of languages with the current languages set
func (langSet LanguageSet) Merge(languages LanguageSet) {
	for languageName := range languages {
		langSet.Add(languageName)
	}
}

// String prints the languages of the language set in sorted order in a comma-separated string format
func (langSet LanguageSet) String() string {
	langNames := make([]string, 0, len(langSet))
	for name := range langSet {
		langNames = append(langNames, name)
	}
	sort.Strings(langNames)
	return strings.Join(langNames, ",")
}
