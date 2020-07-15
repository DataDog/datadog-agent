// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

func Test_resourcequotaTransformer(t *testing.T) {
	type args struct {
		name   string
		metric ksmstore.DDMetric
		tags   []string
	}
	type metricsExpected struct {
		val  float64
		name string
		tags []string
	}
	tests := []struct {
		name     string
		args     args
		expected *metricsExpected
	}{
		{
			name: "nominal case, limit",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 15000,
					Labels: map[string]string{
						"resource":      "pods",
						"type":          "hard",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.resourcequota.pods.limit",
				val:  15000,
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
		},
		{
			name: "nominal case, used",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 7,
					Labels: map[string]string{
						"resource":      "pods",
						"type":          "used",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: &metricsExpected{
				name: "kubernetes_state.resourcequota.pods.used",
				val:  7,
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
		},
		{
			name: "no resource label",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 7,
					Labels: map[string]string{
						"type":          "used",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: nil,
		},
		{
			name: "no type label",
			args: args{
				name: "kube_resourcequota",
				metric: ksmstore.DDMetric{
					Val: 7,
					Labels: map[string]string{
						"resource":      "pods",
						"resourcequota": "gke-resource-quotas",
					},
				},
				tags: []string{"resourcequota:gke-resource-quotas", "foo:bar"},
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		s := mocksender.NewMockSender("ksm")
		s.SetupAcceptAll()
		t.Run(tt.name, func(t *testing.T) {
			resourcequotaTransformer(s, tt.args.name, tt.args.metric, tt.args.tags)
			if tt.expected != nil {
				s.AssertMetric(t, "Gauge", tt.expected.name, tt.expected.val, "", tt.expected.tags)
				s.AssertNumberOfCalls(t, "Gauge", 1)
			} else {
				s.AssertNotCalled(t, "Gauge")
			}
		})
	}
}
