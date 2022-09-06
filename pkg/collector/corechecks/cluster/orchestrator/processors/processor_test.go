// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package processors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Item struct {
	UID string
}

func TestChunkItems(t *testing.T) {
	items := []interface{}{
		Item{UID: "1"},
		Item{UID: "2"},
		Item{UID: "3"},
		Item{UID: "4"},
		Item{UID: "5"},
	}
	expected := [][]interface{}{
		{
			Item{UID: "1"},
			Item{UID: "2"},
		},
		{
			Item{UID: "3"},
			Item{UID: "4"},
		},
		{
			Item{UID: "5"},
		},
	}

	actual := chunkResources(items, 3, 2)
	assert.ElementsMatch(t, expected, actual)
}

func TestSortedMarshal(t *testing.T) {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Annotations: map[string]string{
				"b-annotation":   "test",
				"ab-annotation":  "test",
				"a-annotation":   "test",
				"ac-annotation":  "test",
				"ba-annotation":  "test",
				"1ab-annotation": "test",
			},
		},
	}
	yaml, err := json.Marshal(p)
	assert.NoError(t, err)

	/*	Expected order should be :
			"1ab-annotation": "test",
		    "a-annotation":   "test",
			"ab-annotation":  "test",
			"ac-annotation":  "test",
			"b-annotation":   "test",
			"ba-annotation":  "test",
	*/
	expectedYaml := `{"metadata":{"name":"test-pod","creationTimestamp":null,"annotations":{"1ab-annotation":"test","a-annotation":"test","ab-annotation":"test","ac-annotation":"test","b-annotation":"test","ba-annotation":"test"}},"spec":{"containers":null},"status":{}}`
	actualYaml := string(yaml)
	assert.Equal(t, expectedYaml, actualYaml)
}
