// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common"
)

func TestPromAnnotationsDiffer(t *testing.T) {
	tests := []struct {
		name   string
		checks []*common.PrometheusCheck
		first  map[string]string
		second map[string]string
		want   bool
	}{
		{
			name:   "scrape annotation changed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"prometheus.io/scrape": "false"},
			want:   true,
		},
		{
			name:   "scrape annotation unchanged",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"prometheus.io/scrape": "true"},
			want:   false,
		},
		{
			name:   "scrape annotation removed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/scrape": "true"},
			second: map[string]string{"foo": "bar"},
			want:   true,
		},
		{
			name:   "path annotation changed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/path": "/metrics"},
			second: map[string]string{"prometheus.io/path": "/metrics_custom"},
			want:   true,
		},
		{
			name:   "path annotation unchanged",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/path": "/metrics"},
			second: map[string]string{"prometheus.io/path": "/metrics"},
			want:   false,
		},
		{
			name:   "port annotation changed",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/port": "1234"},
			second: map[string]string{"prometheus.io/port": "4321"},
			want:   true,
		},
		{
			name:   "port annotation unchanged",
			checks: []*common.PrometheusCheck{common.DefaultPrometheusCheck},
			first:  map[string]string{"prometheus.io/port": "1234"},
			second: map[string]string{"prometheus.io/port": "1234"},
			want:   false,
		},
		{
			name:   "include annotation changed",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Incl: map[string]string{"include": "true"}}}}},
			first:  map[string]string{"include": "true"},
			second: map[string]string{"include": "foo"},
			want:   true,
		},
		{
			name:   "include annotation unchanged",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Incl: map[string]string{"include": "true"}}}}},
			first:  map[string]string{"include": "true"},
			second: map[string]string{"include": "true"},
			want:   false,
		},
		{
			name:   "exclude annotation changed",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Excl: map[string]string{"exclude": "true"}}}}},
			first:  map[string]string{"exclude": "true"},
			second: map[string]string{"exclude": "foo"},
			want:   true,
		},
		{
			name:   "exclude annotation unchanged",
			checks: []*common.PrometheusCheck{{AD: &common.ADConfig{KubeAnnotations: &common.InclExcl{Excl: map[string]string{"exclude": "true"}}}}},
			first:  map[string]string{"exclude": "true"},
			second: map[string]string{"exclude": "true"},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PrometheusServicesConfigProvider{}
			p.checks = tt.checks
			if got := p.promAnnotationsDiffer(tt.first, tt.second); got != tt.want {
				t.Errorf("PrometheusServicesConfigProvider.promAnnotationsDiffer() = %v, want %v", got, tt.want)
			}
		})
	}
}
