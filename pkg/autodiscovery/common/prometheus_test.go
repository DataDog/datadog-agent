// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name: "nominal case",
			annotations: map[string]string{
				"foo": "bar",
			},
			want: "http://%%host%%:%%port%%/metrics",
		},
		{
			name: "custom port",
			annotations: map[string]string{
				"foo":                "bar",
				"prometheus.io/port": "1337",
			},
			want: "http://%%host%%:1337/metrics",
		},
		{
			name: "custom path",
			annotations: map[string]string{
				"foo":                "bar",
				"prometheus.io/path": "/metrix",
			},
			want: "http://%%host%%:%%port%%/metrix",
		},
		{
			name: "custom port and path",
			annotations: map[string]string{
				"foo":                "bar",
				"prometheus.io/port": "1337",
				"prometheus.io/path": "/metrix",
			},
			want: "http://%%host%%:1337/metrix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildURL(tt.annotations); got != tt.want {
				t.Errorf("buildURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerRegex(t *testing.T) {
	ad := &ADConfig{}
	ad.setContainersRegex()
	assert.Nil(t, ad.ContainersRe)
	assert.True(t, ad.matchContainer("a-random-string"))

	ad = &ADConfig{KubeContainerNames: []string{"foo"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.ContainersRe)
	assert.True(t, ad.matchContainer("foo"))
	assert.False(t, ad.matchContainer("bar"))

	ad = &ADConfig{KubeContainerNames: []string{"foo", "b*"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.ContainersRe)
	assert.True(t, ad.matchContainer("foo"))
	assert.True(t, ad.matchContainer("bar"))
}

func TestGetPrometheusIncludeAnnotations(t *testing.T) {
	tests := []struct {
		name   string
		config []*PrometheusCheck
		want   map[string]string
	}{
		{
			name:   "empty config, default",
			config: []*PrometheusCheck{},
			want:   PrometheusAnnotations{"prometheus.io/scrape": "true"},
		},
		{
			name:   "custom config",
			config: []*PrometheusCheck{{AD: &ADConfig{KubeAnnotations: &InclExcl{Incl: map[string]string{"include": "true"}}}}},
			want:   PrometheusAnnotations{"include": "true"},
		},
		{
			name: "multiple configs",
			config: []*PrometheusCheck{
				{
					AD: &ADConfig{KubeAnnotations: &InclExcl{Incl: map[string]string{"foo": "bar"}}},
				},
				{
					AD: &ADConfig{KubeAnnotations: &InclExcl{Incl: map[string]string{"baz": "tar"}}},
				},
			},
			want: PrometheusAnnotations{"foo": "bar", "baz": "tar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog.Set("prometheus_scrape.checks", tt.config)
			assert.EqualValues(t, tt.want, GetPrometheusIncludeAnnotations())
		})
	}
}

func TestPrometheusIsMatchingAnnotations(t *testing.T) {
	tests := []struct {
		name           string
		promInclAnnot  PrometheusAnnotations
		svcAnnotations map[string]string
		want           bool
	}{
		{
			name:           "is prom service",
			promInclAnnot:  PrometheusAnnotations{"prometheus.io/scrape": "true"},
			svcAnnotations: map[string]string{"prometheus.io/scrape": "true", "foo": "bar"},
			want:           true,
		},
		{
			name:           "is not prom service",
			promInclAnnot:  PrometheusAnnotations{"prometheus.io/scrape": "true"},
			svcAnnotations: map[string]string{"foo": "bar"},
			want:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.promInclAnnot.IsMatchingAnnotations(tt.svcAnnotations))
		})
	}
}

func TestPrometheusAnnotationsDiffer(t *testing.T) {
	tests := []struct {
		name          string
		promInclAnnot PrometheusAnnotations
		first         map[string]string
		second        map[string]string
		want          bool
	}{
		{
			name:          "differ",
			promInclAnnot: PrometheusAnnotations{"prometheus.io/scrape": "true"},
			first:         map[string]string{"prometheus.io/scrape": "true", "foo": "bar"},
			second:        map[string]string{"prometheus.io/scrape": "false", "foo": "bar"},
			want:          true,
		},
		{
			name:          "no differ",
			promInclAnnot: PrometheusAnnotations{"prometheus.io/scrape": "true"},
			first:         map[string]string{"prometheus.io/scrape": "true", "foo": "bar"},
			second:        map[string]string{"prometheus.io/scrape": "true", "baz": "tar"},
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.promInclAnnot.AnnotationsDiffer(tt.first, tt.second))
		})
	}
}
