// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"reflect"
)

////////////////////////////////
//                            //
//        Language Set        //
//                            //
////////////////////////////////

// Language represents a language name
type Language string

// LanguageSet handles storing sets of languages
type LanguageSet map[Language]struct{}

// Add adds a new language to the language set
// returns false if the language is already
// included in the set, and true otherwise
func (s LanguageSet) Add(language Language) bool {
	_, found := s[language]
	s[language] = struct{}{}
	return !found
}

// Has returns whether the set contains a specific language
func (s LanguageSet) Has(language Language) bool {
	_, found := s[language]
	return found
}

// Remove deletes a language from the language set
func (s LanguageSet) Remove(language Language) {
	delete(s, language)
}

// Merge merges another language set with the current language set
func (s LanguageSet) Merge(other LanguageSet) {
	if len(other) == 0 {
		return
	}

	for language := range other {
		s.Add(language)
	}
}

// EqualTo determines if the current languageset has the same languages
// as another languageset
func (s LanguageSet) EqualTo(other LanguageSet) bool {
	return reflect.DeepEqual(s, other)
}

// ToProto returns a proto message Language
func (s LanguageSet) ToProto() []*pbgo.Language {
	res := make([]*pbgo.Language, 0, len(s))
	for lang := range s {
		res = append(res, &pbgo.Language{
			Name: string(lang),
		})
	}
	return res
}
