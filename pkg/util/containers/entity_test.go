// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildEntityName(t *testing.T) {
	for nb, tc := range []struct {
		runtime  string
		cID      string
		expected string
	}{
		// Empty
		{"", "", ""},
		// Empty cID
		{"docker", "", ""},
		// Empty runtime
		{"", "5bef08742407ef", ""},
		// OK
		{"docker", "5bef08742407ef", "docker://5bef08742407ef"},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.expected), func(t *testing.T) {
			out := BuildEntityName(tc.runtime, tc.cID)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func TestBuildTaggerEntityName(t *testing.T) {
	for nb, tc := range []struct {
		cID      string
		expected string
	}{
		// Empty
		{"", ""},
		// Empty runtime
		{"5bef08742407ef", "container_id://5bef08742407ef"},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.expected), func(t *testing.T) {
			out := BuildTaggerEntityName(tc.cID)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func TestSplitEntityName(t *testing.T) {
	for nb, tc := range []struct {
		entity         string
		expectedPrefix string
		expectedCID    string
	}{
		// OK
		{"container_id://5bef08742407ef", "container_id", "5bef08742407ef"},
		{"container_id://5bef08742407ef", "container_id", "5bef08742407ef"},
		// Invalid
		{"5bef08742407ef", "", ""},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.entity), func(t *testing.T) {
			// Test main method
			r1, c1 := SplitEntityName(tc.entity)
			assert.Equal(t, tc.expectedPrefix, r1)
			assert.Equal(t, tc.expectedCID, c1)

			// Test the wraper
			c2 := ContainerIDForEntity(tc.entity)
			assert.Equal(t, tc.expectedCID, c2)
		})
	}
}
