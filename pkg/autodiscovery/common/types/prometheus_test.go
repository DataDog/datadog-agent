// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"testing"

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
			if got := BuildURL(tt.annotations); got != tt.want {
				t.Errorf("buildURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerRegex(t *testing.T) {
	ad := &ADConfig{}
	ad.setContainersRegex()
	assert.Nil(t, ad.ContainersRe)
	assert.True(t, ad.MatchContainer("a-random-string"))

	ad = &ADConfig{KubeContainerNames: []string{"foo"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.ContainersRe)
	assert.True(t, ad.MatchContainer("foo"))
	assert.False(t, ad.MatchContainer("bar"))

	ad = &ADConfig{KubeContainerNames: []string{"foo", "b*"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.ContainersRe)
	assert.True(t, ad.MatchContainer("foo"))
	assert.True(t, ad.MatchContainer("bar"))
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
