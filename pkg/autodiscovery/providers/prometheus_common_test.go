// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package providers

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
			if got := buildURL(tt.annotations); got != tt.want {
				t.Errorf("buildURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainerRegex(t *testing.T) {
	ad := &ADConfig{}
	ad.setContainersRegex()
	assert.Nil(t, ad.containersRe)
	assert.True(t, ad.matchContainer("a-random-string"))

	ad = &ADConfig{KubeContainerNames: []string{"foo"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.containersRe)
	assert.True(t, ad.matchContainer("foo"))
	assert.False(t, ad.matchContainer("bar"))

	ad = &ADConfig{KubeContainerNames: []string{"foo", "b*"}}
	ad.setContainersRegex()
	assert.NotNil(t, ad.containersRe)
	assert.True(t, ad.matchContainer("foo"))
	assert.True(t, ad.matchContainer("bar"))
}
