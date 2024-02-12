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
)

////////////////////////////////
//                            //
//     Language Set Tests     //
//                            //
////////////////////////////////

func TestLanguageSetOperations(t *testing.T) {

	tests := []struct {
		name      string
		baseSet   LanguageSet
		operation func(LanguageSet)
		expected  LanguageSet
	}{
		{
			name:      "add item to language set",
			baseSet:   LanguageSet{},
			operation: func(set LanguageSet) { set.Add("java") },
			expected:  LanguageSet{"java": struct{}{}},
		},
		{
			name:    "add multiple items to language set",
			baseSet: LanguageSet{},
			operation: func(set LanguageSet) {
				set.Add("java")
				set.Add("cpp")
			},
			expected: LanguageSet{"java": struct{}{}, "cpp": struct{}{}},
		},
		{
			name:      "delete existing language from language set",
			baseSet:   LanguageSet{"java": struct{}{}},
			operation: func(set LanguageSet) { set.Remove("java") },
			expected:  LanguageSet{},
		},
		{
			name:      "delete a non existing language from language set should not return an error",
			baseSet:   LanguageSet{},
			operation: func(set LanguageSet) { set.Remove("java") },
			expected:  LanguageSet{},
		},
		{
			name:    "merge with another languageset",
			baseSet: LanguageSet{},
			operation: func(set LanguageSet) {
				other := LanguageSet{"java": struct{}{}, "cpp": struct{}{}}
				set.Merge(other)
			},
			expected: LanguageSet{"java": struct{}{}, "cpp": struct{}{}},
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
		baseSet    LanguageSet
		target     Language
		shouldHave bool
	}{
		{
			name:       "has existing item",
			baseSet:    LanguageSet{"java": {}},
			target:     "java",
			shouldHave: true,
		},
		{
			name:       "should not have missing item",
			baseSet:    LanguageSet{"java": {}},
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
