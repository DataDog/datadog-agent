// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"reflect"
	"time"
)

// Language represents a language name
type Language string

////////////////////////////////
//                            //
//        Language Set        //
//                            //
////////////////////////////////

// LanguageSet represents a set of languages
type LanguageSet map[Language]struct{}

// Add adds a new language to the language set
// returns false if the language is already included in the set, and true otherwise
func (s LanguageSet) Add(language Language) bool {
	_, found := s[language]
	s[language] = struct{}{}
	return !found
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

////////////////////////////////
//                            //
//     Timed Language Set     //
//                            //
////////////////////////////////

// TimedLanguageSet handles storing sets of languages along with their expiration times
type TimedLanguageSet map[Language]time.Time

// RemoveExpired removes all expired languages from the set
// Returns true if at least one language is expired and removed
func (s TimedLanguageSet) RemoveExpired() bool {
	removedAtLeastOne := false
	for lang, expiration := range s {
		if expiration.Before(time.Now()) {
			s.Remove(lang)
			removedAtLeastOne = true
		}
	}
	return removedAtLeastOne
}

// Add adds a new language to the language set with an expiration time
// returns false if the language is already included in the set, and true otherwise
func (s TimedLanguageSet) Add(language Language, expiration time.Time) bool {
	_, found := s[language]
	s[language] = expiration
	return !found
}

// Has returns whether the set contains a specific language
func (s TimedLanguageSet) Has(language Language) bool {
	_, found := s[language]
	return found
}

// Remove deletes a language from the language set
func (s TimedLanguageSet) Remove(language Language) {
	delete(s, language)
}

// Merge merges another timed language set with the current language set
// returns true if the set new languages were introduced, and false otherwise
func (s TimedLanguageSet) Merge(other TimedLanguageSet) bool {

	modified := false

	for language, expiration := range other {
		if s.Add(language, expiration) {
			modified = true
		}
	}

	return modified
}

// EqualTo determines if the current timed languageset has the same languages
// as another timed languageset
func (s TimedLanguageSet) EqualTo(other TimedLanguageSet) bool {
	return reflect.DeepEqual(s, other)
}
