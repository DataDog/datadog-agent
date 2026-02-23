// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagemodels

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

//////////////////////////////////////////
//                                      //
//      ContainersLanguages Tests       //
//                                      //
//////////////////////////////////////////

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
			name: "Non-empty containers languages",
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

//////////////////////////////////////////
//                                      //
//    TimedContainersLanguages Tests    //
//                                      //
//////////////////////////////////////////

func TestContainersLanguagesGetOrInitialize(t *testing.T) {
	mockExpiration := time.Now()

	tests := []struct {
		name               string
		containerLanguages TimedContainersLanguages
		container          Container
		expected           *TimedLanguageSet
	}{
		{
			name:               "missing existing container should get initialized",
			containerLanguages: make(TimedContainersLanguages),
			container:          *NewContainer("some-container"),
			expected:           &TimedLanguageSet{},
		},
		{
			name:               "should return language set for existing container",
			containerLanguages: map[Container]TimedLanguageSet{*NewContainer("some-container"): {"java": mockExpiration}},
			container:          *NewContainer("some-container"),
			expected:           &TimedLanguageSet{"java": mockExpiration},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.True(tt, reflect.DeepEqual(test.containerLanguages.GetOrInitialize(test.container), test.expected), fmt.Sprintf("Expected %v, found %v", test.expected, test.container))
		})
	}
}

func TestTimedContainersLanguagesMerge(t *testing.T) {
	mockExpiration := time.Now()

	tests := []struct {
		name               string
		containerLanguages TimedContainersLanguages
		other              TimedContainersLanguages
		expectedAfterMerge TimedContainersLanguages
	}{
		{
			name:               "merge empty containers languages",
			containerLanguages: make(TimedContainersLanguages),
			other:              make(TimedContainersLanguages),
			expectedAfterMerge: make(TimedContainersLanguages),
		},
		{
			name:               "merge non-empty other container to empty self",
			containerLanguages: make(TimedContainersLanguages),
			other:              TimedContainersLanguages{*NewContainer("some-container"): {"java": mockExpiration}},
			expectedAfterMerge: TimedContainersLanguages{*NewContainer("some-container"): {"java": mockExpiration}},
		},
		{
			name:               "merge empty other container to non-empty self",
			containerLanguages: TimedContainersLanguages{*NewContainer("some-container"): {"java": mockExpiration}},
			other:              make(TimedContainersLanguages),
			expectedAfterMerge: TimedContainersLanguages{*NewContainer("some-container"): {"java": mockExpiration}},
		},
		{
			name: "merge non-empty other container to non-empty self",
			containerLanguages: TimedContainersLanguages{
				*NewContainer("some-container"):          {"java": mockExpiration},
				*NewInitContainer("some-init-container"): {"go": mockExpiration},
			},
			other: TimedContainersLanguages{
				*NewContainer("some-other-container"): {"ruby": mockExpiration},
				*NewContainer("some-container"):       {"cpp": mockExpiration, "java": mockExpiration},
			},
			expectedAfterMerge: TimedContainersLanguages{
				*NewContainer("some-other-container"):    {"ruby": mockExpiration},
				*NewContainer("some-container"):          {"cpp": mockExpiration, "java": mockExpiration},
				*NewInitContainer("some-init-container"): {"go": mockExpiration},
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
		self           TimedContainersLanguages
		other          TimedContainersLanguages
		expectAreEqual bool
	}{
		{
			name:           "Should not be equal to nil",
			self:           make(TimedContainersLanguages),
			other:          nil,
			expectAreEqual: false,
		},
		{
			name: "equality test",
			self: TimedContainersLanguages{
				*NewContainer("cont-1"):     {"java": {}, "python": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			other: TimedContainersLanguages{
				*NewContainer("cont-1"):     {"python": {}, "java": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			expectAreEqual: true,
		},
		{
			name: "inequality test",
			self: TimedContainersLanguages{
				*NewContainer("cont-1"):     {"java": {}, "python": {}},
				*NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			other: TimedContainersLanguages{
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

func TestToProto(t *testing.T) {
	cl := ContainersLanguages{
		*NewContainer("web"):         {"java": {}, "python": {}},
		*NewInitContainer("db-init"): {"go": {}},
		*NewContainer("worker"):      {"ruby": {}},
	}

	containers, initContainers := cl.ToProto()

	// Should have 2 standard containers and 1 init container
	assert.Len(t, containers, 2)
	assert.Len(t, initContainers, 1)
	assert.Equal(t, "db-init", initContainers[0].ContainerName)

	// Verify standard containers contain expected names
	containerNames := make(map[string]bool)
	for _, c := range containers {
		containerNames[c.ContainerName] = true
	}
	assert.True(t, containerNames["web"])
	assert.True(t, containerNames["worker"])
}

func TestEqualToDifferentContainers(t *testing.T) {
	// Same length but different container names
	a := TimedContainersLanguages{
		*NewContainer("cont-a"): {"java": {}},
	}
	b := TimedContainersLanguages{
		*NewContainer("cont-b"): {"java": {}},
	}
	assert.False(t, a.EqualTo(b))
}

func TestRemoveExpiredLanguagesNoneExpired(t *testing.T) {
	mockTime := time.Now()
	containersLanguages := TimedContainersLanguages{
		*NewContainer("cont"): {"java": mockTime.Add(10 * time.Minute)},
	}
	removedAny := containersLanguages.RemoveExpiredLanguages()
	assert.False(t, removedAny)
	assert.Len(t, containersLanguages, 1)
}

func TestMergeReturnValue(t *testing.T) {
	mockExpiration := time.Now()

	cl := TimedContainersLanguages{
		*NewContainer("cont"): {"java": mockExpiration},
	}

	// Merging with same content should not report modifications
	same := TimedContainersLanguages{
		*NewContainer("cont"): {"java": mockExpiration},
	}
	modified := cl.Merge(same)
	assert.False(t, modified)

	// Merging with new content should report modifications
	different := TimedContainersLanguages{
		*NewContainer("cont"): {"python": mockExpiration},
	}
	modified = cl.Merge(different)
	assert.True(t, modified)
}

func TestRemoveExpiredLanguages(t *testing.T) {
	mockTime := time.Now()
	mixedlangset := TimedLanguageSet{"java": mockTime.Add(10 * time.Minute), "cpp": mockTime.Add(-10 * time.Minute)}
	expiredlangset := TimedLanguageSet{"java": mockTime.Add(-10 * time.Minute), "cpp": mockTime.Add(-10 * time.Minute)}
	containersLanguages := TimedContainersLanguages{
		*NewContainer("cont-name"):         mixedlangset,
		*NewContainer("another-cont-name"): expiredlangset,
	}
	removedAny := containersLanguages.RemoveExpiredLanguages()
	assert.True(t, removedAny)

	expectedLangset := TimedLanguageSet{"java": mockTime.Add(10 * time.Minute)}
	expectedTimedContainersLanguages := TimedContainersLanguages{*NewContainer("cont-name"): expectedLangset}
	assert.Truef(t, reflect.DeepEqual(containersLanguages, expectedTimedContainersLanguages), "Expected %v, found %v", expectedTimedContainersLanguages, containersLanguages)
}
