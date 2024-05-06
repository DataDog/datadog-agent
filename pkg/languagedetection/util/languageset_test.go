// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"
)

////////////////////////////////
//                            //
//     Language Set Tests     //
//                            //
////////////////////////////////

func TestAddToLanguageSet(t *testing.T) {
	s := LanguageSet{}

	added := s.Add("cpp")
	assert.True(t, added)

	expectedAfterAdd := LanguageSet{"cpp": {}}
	assert.Truef(t, reflect.DeepEqual(s, expectedAfterAdd), "Expected %v, found %v", expectedAfterAdd, s)

	added = s.Add("cpp")
	assert.False(t, added)
	assert.Truef(t, reflect.DeepEqual(s, expectedAfterAdd), "Expected %v, found %v", expectedAfterAdd, s)
}

////////////////////////////////
//                            //
//    TimedLanguageSet Set    //
//                            //
////////////////////////////////

func TestTimedLanguageSetOperations(t *testing.T) {
	mockExpiration := time.Now()

	tests := []struct {
		name      string
		baseSet   TimedLanguageSet
		operation func(TimedLanguageSet)
		expected  TimedLanguageSet
	}{
		{
			name:      "add item to language set",
			baseSet:   TimedLanguageSet{},
			operation: func(set TimedLanguageSet) { set.Add("java", mockExpiration) },
			expected:  TimedLanguageSet{"java": mockExpiration},
		},
		{
			name:    "add multiple items to language set",
			baseSet: TimedLanguageSet{"java": mockExpiration.Add(-2 * time.Second)},
			operation: func(set TimedLanguageSet) {
				set.Add("java", mockExpiration)
				set.Add("cpp", mockExpiration)
			},
			expected: TimedLanguageSet{"java": mockExpiration, "cpp": mockExpiration},
		},
		{
			name:      "delete existing language from language set",
			baseSet:   TimedLanguageSet{"java": mockExpiration},
			operation: func(set TimedLanguageSet) { set.Remove("java") },
			expected:  TimedLanguageSet{},
		},
		{
			name:      "delete a non existing language from language set should not return an error",
			baseSet:   TimedLanguageSet{},
			operation: func(set TimedLanguageSet) { set.Remove("java") },
			expected:  TimedLanguageSet{},
		},
		{
			name:    "merge with another languageset",
			baseSet: TimedLanguageSet{"java": mockExpiration.Add(-2 * time.Second)},
			operation: func(set TimedLanguageSet) {
				other := TimedLanguageSet{"java": mockExpiration, "cpp": mockExpiration}
				set.Merge(other)
			},
			expected: TimedLanguageSet{"java": mockExpiration, "cpp": mockExpiration},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			test.operation(test.baseSet)
			assert.True(tt, reflect.DeepEqual(test.baseSet, test.expected), fmt.Sprintf("Expected %v, found %v", test.expected, test.baseSet))
		})
	}
}

func TestHas(t *testing.T) {

	tests := []struct {
		name       string
		baseSet    TimedLanguageSet
		target     Language
		shouldHave bool
	}{
		{
			name:       "has existing item",
			baseSet:    TimedLanguageSet{"java": {}},
			target:     "java",
			shouldHave: true,
		},
		{
			name:       "should not have missing item",
			baseSet:    TimedLanguageSet{"java": {}},
			target:     "cpp",
			shouldHave: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			hasItem := test.baseSet.Has(test.target)
			if test.shouldHave {
				assert.Truef(tt, hasItem, "set should have %v", test.target)
			} else {
				assert.Falsef(tt, hasItem, "set should not have %v", test.target)
			}
		})
	}
}

func TestRemoveExpired(t *testing.T) {
	mockTime := time.Now()
	langset := TimedLanguageSet{"java": mockTime.Add(10 * time.Minute), "cpp": mockTime.Add(-10 * time.Minute)}
	removedAny := langset.RemoveExpired()
	assert.True(t, removedAny)

	expectedLangset := TimedLanguageSet{"java": mockTime.Add(10 * time.Minute)}
	assert.Truef(t, reflect.DeepEqual(langset, expectedLangset), "Expected %v, found %v", expectedLangset, langset)
}
