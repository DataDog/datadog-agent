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
// Containers Languages Tests //
//                            //
////////////////////////////////

func TestContainersLanguagesGetOrInitialize(t *testing.T) {
	tests := []struct {
		name               string
		containerLanguages ContainersLanguages
		container          Container
		expected           *LanguageSet
	}{
		{
			name:               "missing existing container should get initialized",
			containerLanguages: make(ContainersLanguages),
			container:          *NewContainer("some-container"),
			expected:           &LanguageSet{},
		},
		{
			name:               "should return language set for existing container",
			containerLanguages: map[Container]LanguageSet{*NewContainer("some-container"): {"java": struct{}{}}},
			container:          *NewContainer("some-container"),
			expected:           &LanguageSet{"java": struct{}{}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.True(tt, reflect.DeepEqual(test.containerLanguages.GetOrInitialize(test.container), test.expected), fmt.Sprintf("Expected %v, found %v", test.expected, test.container))
		})
	}
}

func TestContainersLanguagesMerge(t *testing.T) {
	tests := []struct {
		name               string
		containerLanguages ContainersLanguages
		other              ContainersLanguages
		expectedAfterMerge ContainersLanguages
	}{
		{
			name:               "merge empty containers languages",
			containerLanguages: make(ContainersLanguages),
			other:              make(ContainersLanguages),
			expectedAfterMerge: make(ContainersLanguages),
		},
		{
			name:               "merge non-empty other container to empty self",
			containerLanguages: make(ContainersLanguages),
			other:              map[Container]LanguageSet{*NewContainer("some-container"): {"java": struct{}{}}},
			expectedAfterMerge: map[Container]LanguageSet{*NewContainer("some-container"): {"java": struct{}{}}},
		},
		{
			name:               "merge empty other container to non-empty self",
			containerLanguages: map[Container]LanguageSet{*NewContainer("some-container"): {"java": struct{}{}}},
			other:              make(ContainersLanguages),
			expectedAfterMerge: map[Container]LanguageSet{*NewContainer("some-container"): {"java": struct{}{}}},
		},
		{
			name: "merge non-empty other container to non-empty self",
			containerLanguages: map[Container]LanguageSet{
				*NewContainer("some-container"):          {"java": struct{}{}},
				*NewInitContainer("some-init-container"): {"go": struct{}{}},
			},
			other: map[Container]LanguageSet{
				*NewContainer("some-other-container"): {"ruby": struct{}{}},
				*NewContainer("some-container"):       {"cpp": struct{}{}, "java": struct{}{}},
			},
			expectedAfterMerge: map[Container]LanguageSet{
				*NewContainer("some-other-container"):    {"ruby": struct{}{}},
				*NewContainer("some-container"):          {"cpp": struct{}{}, "java": struct{}{}},
				*NewInitContainer("some-init-container"): {"go": struct{}{}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			test.containerLanguages.Merge(test.other)
			assert.True(tt, reflect.DeepEqual(test.containerLanguages, test.expectedAfterMerge), fmt.Sprintf("Expected %v, found %v", test.expectedAfterMerge, test.containerLanguages))
		})
	}
}

func TestEqualTo(t *testing.T) {

	tests := []struct {
		name           string
		self           ContainersLanguages
		other          ContainersLanguages
		expectAreEqual bool
	}{
		{
			name:           "Should not be equal to nil",
			self:           make(ContainersLanguages),
			other:          nil,
			expectAreEqual: false,
		},
		{
			name: "equality test",
			self: ContainersLanguages{
				*NewContainer("cont-1"):     {"java": {}, "python": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			other: ContainersLanguages{
				*NewContainer("cont-1"):     {"python": {}, "java": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			expectAreEqual: true,
		},
		{
			name: "inequality test",
			self: ContainersLanguages{
				*NewContainer("cont-1"):     {"java": {}, "python": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			other: ContainersLanguages{
				*NewContainer("cont-1"):     {"python": {}, "java": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
				*NewContainer("intruder"):   {"cpp": {}},
			},
			expectAreEqual: false,
		},
	}

	for _, test := range tests {

		t.Run(test.name, func(tt *testing.T) {
			result := test.self.EqualTo(test.other)

			if test.expectAreEqual {
				assert.Truef(tt, result, "expected to be equal, found: not equal")
			} else {
				assert.Falsef(tt, result, "expected not to be equal, found: equal")
			}
		})
	}
}

func TestToAnnotations(t *testing.T) {
	tests := []struct {
		name                string
		self                ContainersLanguages
		expectedAnnotations map[string]string
	}{
		{
			name:                "Empty containers languages",
			self:                make(ContainersLanguages),
			expectedAnnotations: map[string]string{},
		},
		{
			name: "Empty containers languages",
			self: ContainersLanguages{
				*NewContainer("cont-1"):     {"java": {}, "python": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			expectedAnnotations: map[string]string{
				"internal.dd.datadoghq.com/cont-1.detected_langs":      "java,python",
				"internal.dd.datadoghq.com/init.cont-2.detected_langs": "java,python",
			},
		},
	}

	for _, test := range tests {

		t.Run(test.name, func(tt *testing.T) {
			annotations := test.self.ToAnnotations()
			assert.Truef(tt, reflect.DeepEqual(test.expectedAnnotations, annotations), "expected %v, found %v", test.expectedAnnotations, annotations)
		})
	}

}
