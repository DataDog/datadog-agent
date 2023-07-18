// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

func Test_labelJoiner(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]*joinsConfig
		families map[string][]ksmstore.DDMetricsFam
		expected []struct {
			inputLabels map[string]string
			labelsToAdd []label
		}
	}{
		{
			name: "One label to match, one label to get",
			config: map[string]*joinsConfig{
				"kube_pod_info": {
					labelsToMatch: []string{"foo_key"},
					labelsToGet:   map[string]string{"qux_key": "qux_tag"},
				},
			},
			families: map[string][]ksmstore.DDMetricsFam{
				"uuid1": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value1",
									"qux_key":  "qux_value1",
									"quux_key": "quux_value1",
								},
							},
						},
					},
				},
				"uuid2": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value2",
									"qux_key":  "qux_value2",
									"quux_key": "quux_value2",
								},
							},
						},
					},
				},
			},
			expected: []struct {
				inputLabels map[string]string
				labelsToAdd []label
			}{
				{
					inputLabels: map[string]string{"foo_key": "foo_different_value"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{"foo_key": "foo_value1"},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value1"},
					},
				},
				{
					inputLabels: map[string]string{"foo_key": "foo_value2"},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value2"},
					},
				},
			},
		},
		{
			name: "Two labels to match, two labels to get",
			config: map[string]*joinsConfig{
				"kube_pod_info": {
					labelsToMatch: []string{"foo_key", "bar_key"},
					labelsToGet:   map[string]string{"qux_key": "qux_tag", "quux_key": "quux_tag"},
				},
			},
			families: map[string][]ksmstore.DDMetricsFam{
				"uuid1": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value1",
									"bar_key":  "bar_value1",
									"qux_key":  "qux_value1",
									"quux_key": "quux_value1",
								},
							},
						},
					},
				},
				"uuid2": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value2",
									"bar_key":  "bar_value2",
									"qux_key":  "qux_value2",
									"quux_key": "quux_value2",
								},
							},
						},
					},
				},
				"uuid12": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value1",
									"bar_key":  "bar_value2",
									"qux_key":  "qux_value12",
									"quux_key": "quux_value12",
								},
							},
						},
					},
				},
			},
			expected: []struct {
				inputLabels map[string]string
				labelsToAdd []label
			}{
				{
					inputLabels: map[string]string{"foo_key": "foo_different_value"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{"foo_key": "foo_value1"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{"bar_key": "bar_value1"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value2",
						"bar_key": "bar_value1",
					},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"bar_key": "bar_value1",
					},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value1"},
						{key: "quux_tag", value: "quux_value1"},
					},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value2",
						"bar_key": "bar_value2",
					},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value2"},
						{key: "quux_tag", value: "quux_value2"},
					},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"bar_key": "bar_value2",
					},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value12"},
						{key: "quux_tag", value: "quux_value12"},
					},
				},
			},
		},
		{
			name: "Three labels to match, all labels to get",
			config: map[string]*joinsConfig{
				"kube_pod_info": {
					labelsToMatch: []string{"foo_key", "bar_key", "baz_key"},
					getAllLabels:  true,
				},
			},
			families: map[string][]ksmstore.DDMetricsFam{
				"uuid1": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value1",
									"bar_key":  "bar_value1",
									"baz_key":  "baz_value1",
									"qux_key":  "qux_value1",
									"quux_key": "quux_value1",
								},
							},
						},
					},
				},
				"uuid2": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value2",
									"bar_key":  "bar_value2",
									"baz_key":  "baz_value2",
									"qux_key":  "qux_value2",
									"quux_key": "quux_value2",
								},
							},
						},
					},
				},
				"uuid12": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value1",
									"bar_key":  "bar_value1",
									"baz_key":  "baz_value2",
									"qux_key":  "qux_value12",
									"quux_key": "quux_value12",
								},
							},
						},
					},
				},
			},
			expected: []struct {
				inputLabels map[string]string
				labelsToAdd []label
			}{
				{
					inputLabels: map[string]string{"foo_key": "foo_different_value"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{"foo_key": "foo_value1"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"bar_key": "bar_value1",
					},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"baz_key": "baz_value1",
					},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"bar_key": "bar_value2",
						"baz_key": "baz_value2",
					},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"bar_key": "bar_value1",
						"baz_key": "baz_value1",
					},
					labelsToAdd: []label{
						{key: "qux_key", value: "qux_value1"},
						{key: "quux_key", value: "quux_value1"},
					},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value2",
						"bar_key": "bar_value2",
						"baz_key": "baz_value2",
					},
					labelsToAdd: []label{
						{key: "qux_key", value: "qux_value2"},
						{key: "quux_key", value: "quux_value2"},
					},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value1",
						"bar_key": "bar_value1",
						"baz_key": "baz_value2",
					},
					labelsToAdd: []label{
						{key: "qux_key", value: "qux_value12"},
						{key: "quux_key", value: "quux_value12"},
					},
				},
			},
		},
		{
			name: "Several metrics with the same value for labels to match",
			config: map[string]*joinsConfig{
				"kube_pod_info": {
					labelsToMatch: []string{"foo_key"},
					labelsToGet:   map[string]string{"qux_key": "qux_tag"},
				},
			},
			families: map[string][]ksmstore.DDMetricsFam{
				"uuid1": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value",
									"qux_key":  "qux_value1",
									"quux_key": "quux_value1",
								},
							},
						},
					},
				},
				"uuid2": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key":  "foo_value",
									"qux_key":  "qux_value2",
									"quux_key": "quux_value2",
								},
							},
						},
					},
				},
			},
			expected: []struct {
				inputLabels map[string]string
				labelsToAdd []label
			}{
				{
					inputLabels: map[string]string{"foo_key": "foo_different_value"},
					labelsToAdd: []label{},
				},
				{
					inputLabels: map[string]string{
						"foo_key": "foo_value",
						"bar_key": "no_matter",
					},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value1"},
						{key: "qux_tag", value: "qux_value2"},
					},
				},
			},
		},
		{
			name: "Skip tags with empty value",
			config: map[string]*joinsConfig{
				"kube_pod_info": {
					labelsToMatch: []string{"foo_key"},
					labelsToGet:   map[string]string{"qux_key": "qux_tag"},
				},
			},
			families: map[string][]ksmstore.DDMetricsFam{
				"uuid1": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key": "foo_value1",
									"qux_key": "qux_value1",
								},
							},
						},
					},
				},
				"uuid2": {
					{
						Name: "kube_pod_info",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{
									"foo_key": "foo_value2",
									"qux_key": "",
								},
							},
						},
					},
				},
			},
			expected: []struct {
				inputLabels map[string]string
				labelsToAdd []label
			}{
				{
					inputLabels: map[string]string{"foo_key": "foo_value1"},
					labelsToAdd: []label{
						{key: "qux_tag", value: "qux_value1"},
					},
				},
				{
					inputLabels: map[string]string{"foo_key": "foo_value2"},
					labelsToAdd: []label{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labelJoiner := newLabelJoiner(tt.config)
			labelJoiner.insertFamilies(tt.families)
			for _, expected := range tt.expected {
				assert.ElementsMatch(t, labelJoiner.getLabelsToAdd(expected.inputLabels), expected.labelsToAdd)
			}
		})
	}
}
