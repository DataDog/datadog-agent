// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localkubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindImagesToLoad(t *testing.T) {
	localImages := map[string]bool{
		"agent:kind-1":                        true,
		"cluster-agent:kind-1":                true,
		"extra:dev":                           true,
		"registry.datadoghq.com/agent:7.85.4": true,
	}
	lookup := func(image string) bool {
		return localImages[image]
	}

	tests := []struct {
		name       string
		requested  []string
		configured []string
		want       []string
	}{
		{
			name: "nothing to load",
			want: nil,
		},
		{
			name:       "configured local images are loaded",
			configured: []string{"agent:kind-1", "cluster-agent:kind-1"},
			want:       []string{"agent:kind-1", "cluster-agent:kind-1"},
		},
		{
			name:       "configured images missing locally or unset are left to the registry",
			configured: []string{"gcr.io/datadoghq/agent:7.85.2", ""},
			want:       nil,
		},
		{
			name:       "configured images that name a registry are left to the registry even when present locally",
			configured: []string{"registry.datadoghq.com/agent:7.85.4"},
			want:       nil,
		},
		{
			name:      "requested images are kept even when missing locally",
			requested: []string{"extra:dev", "missing:dev"},
			want:      []string{"extra:dev", "missing:dev"},
		},
		{
			name:       "an image both requested and configured is loaded once",
			requested:  []string{"cluster-agent:kind-1"},
			configured: []string{"agent:kind-1", "cluster-agent:kind-1"},
			want:       []string{"cluster-agent:kind-1", "agent:kind-1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, kindImagesToLoad(test.requested, test.configured, lookup))
		})
	}
}

func TestNamesRegistry(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{image: "agent:dev", want: false},
		{image: "datadog/agent:7.85.4", want: false},
		{image: "gcr.io/datadoghq/agent:7.85.4", want: true},
		{image: "localhost/agent:dev", want: true},
		{image: "localhost:5000/agent:dev", want: true},
	}

	for _, test := range tests {
		t.Run(test.image, func(t *testing.T) {
			assert.Equal(t, test.want, namesRegistry(test.image))
		})
	}
}
