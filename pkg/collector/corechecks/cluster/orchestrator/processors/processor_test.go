// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package processors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
	json, err := json.Marshal(p)
	assert.NoError(t, err)

	//nolint:revive // TODO(CAPP) Fix revive linter
	expectedJson := `{
						"metadata":{
							"name":"test-pod",
							"creationTimestamp":null,
							"annotations":{
								"1ab-annotation":"test",
								"a-annotation":"test",
								"ab-annotation":"test",
								"ac-annotation":"test",
								"b-annotation":"test",
								"ba-annotation":"test"
							}
						},
						"spec":{
							"containers":null
						},
						"status":{}
					}`
	//nolint:revive // TODO(CAPP) Fix revive linter
	actualJson := string(json)
	assert.JSONEq(t, expectedJson, actualJson)
}

func TestInsertDeletionTimestampIfPossible(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		obj      interface{}
		expected interface{}
	}{
		{
			name:     "nil object",
			obj:      nil,
			expected: nil,
		},
		{
			name:     "non-pointer type",
			obj:      appsv1.ReplicaSet{},
			expected: appsv1.ReplicaSet{},
		},
		{
			name:     "non-struct type",
			obj:      &[]string{},
			expected: &[]string{},
		},
		{
			name: "object without ObjectMeta",
			obj: &struct {
				Name string
			}{Name: "test"},
			expected: &struct {
				Name string
			}{Name: "test"},
		},
		{
			name: "object with existing DeletionTimestamp",
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rs",
					DeletionTimestamp: &metav1.Time{Time: now.Add(-time.Hour)},
				},
			},
			expected: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rs",
					DeletionTimestamp: &metav1.Time{Time: now.Add(-time.Hour)},
				},
			},
		},
		{
			name: "unstructured object",
			obj:  &unstructured.Unstructured{},
			expected: func() interface{} {
				u := &unstructured.Unstructured{}
				u.SetDeletionTimestamp(&metav1.Time{Time: now})
				return u
			}(),
		},
		{
			name: "regular kubernetes object",
			obj: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "rs",
				},
			},
			expected: &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rs",
					DeletionTimestamp: &metav1.Time{Time: now},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertDeletionTimestampIfPossible(tt.obj, now)
			require.Equal(t, tt.expected, result)
		})
	}
}
