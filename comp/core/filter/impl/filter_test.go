// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterimpl contains the implementation of the filter component.
package filterimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	filterdef "github.com/DataDog/datadog-agent/comp/core/filter/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Create a new filter object for testing purposes
func newFilterObject(t *testing.T, config config.Component) *filter {
	reqs := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config,
	}
	f, _ := NewComponent(reqs)
	return f.Comp.(*filter)
}

func TestADContainerFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	f := newFilterObject(t, mockConfig)

	container := filterdef.Container{
		Name: "dd-agent",
		Annotations: map[string]string{
			"ad.datadoghq.com/garbage": "true",
		},
	}
	// Expect to return the default value
	res, err := f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerADAnnotations}, false)
	assert.NoError(t, err)
	assert.Equal(t, false, res)

	container = filterdef.Container{
		Name: "dd-agent",
		Annotations: map[string]string{
			"ad.datadoghq.com/dd-agent.exclude": "true",
		},
	}
	res, err = f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerADAnnotations}, false)
	assert.NoError(t, err)
	assert.Equal(t, true, res)

	// Edge case if the container name is missing
	container = filterdef.Container{
		Annotations: map[string]string{
			"ad.datadoghq.com/.exclude": "true",
		},
	}
	res, err = f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerADAnnotations}, false)
	assert.NoError(t, err)
	assert.Equal(t, true, res)
}

func TestCombinedFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"name:nginx"})
	mockConfig.SetWithoutSource("ac_include", []string{"kube_namespace:default"})
	mockConfig.SetWithoutSource("ac_exclude", []string{"kube_namespace:datadog-agent"})

	f := newFilterObject(t, mockConfig)

	container := filterdef.Container{
		Name: "dd-agent",
	}
	res, err := f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerGlobal}, true)
	assert.NoError(t, err)
	assert.Equal(t, false, res, "Container exclusion result mismatch")

	container = filterdef.Container{
		Name:      "dd-agent",
		Namespace: "default",
	}
	res, err = f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerGlobal, filterdef.ContainerACLegacy}, true)
	assert.NoError(t, err)
	assert.Equal(t, false, res, "Container exclusion result mismatch")

	container = filterdef.Container{
		Name:      "nginx",
		Namespace: "default",
	}
	res, err = f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerGlobal, filterdef.ContainerACLegacy}, true)
	assert.NoError(t, err)
	assert.Equal(t, false, res, "Container exclusion result mismatch")

	container = filterdef.Container{
		Name:      "nginx",
		Namespace: "datadog-agent",
	}
	res, err = f.IsContainerExcluded(container, []filterdef.ContainerFilter{filterdef.ContainerGlobal, filterdef.ContainerACLegacy}, false)
	assert.NoError(t, err)
	assert.Equal(t, true, res, "Container exclusion result mismatch")
}

func TestContainerSBOMFilter(t *testing.T) {

	tests := []struct {
		name         string
		include      []string
		exclude      []string
		pauseCtn     bool
		container    filterdef.Container
		defaultValue bool
		expected     bool
	}{
		{
			name:     "Include image",
			include:  []string{"image:dd-agent"},
			exclude:  []string{"image:nginx"},
			pauseCtn: false,
			container: filterdef.Container{
				Image: "dd-agent",
			},
			defaultValue: true,
			expected:     false,
		},
		{
			name:     "Exclude image",
			include:  []string{"image:dd-agent"},
			exclude:  []string{"image:nginx"},
			pauseCtn: false,
			container: filterdef.Container{
				Image: "nginx-123",
			},
			defaultValue: false,
			expected:     true,
		},
		{
			name:     "Included namespace beats excluded name",
			include:  []string{"kube_namespace:default"},
			exclude:  []string{"name:nginx"},
			pauseCtn: false,
			container: filterdef.Container{
				Name:      "nginx",
				Namespace: "default",
			},
			defaultValue: true,
			expected:     false,
		},
		{
			name:     "Included name beats excluded namespace",
			include:  []string{"name:nginx"},
			exclude:  []string{"kube_namespace:default"},
			pauseCtn: false,
			container: filterdef.Container{
				Name:      "nginx",
				Namespace: "default",
			},
			defaultValue: true,
			expected:     false,
		},
		{
			name:     "Exclude pause container",
			include:  []string{""},
			exclude:  []string{"name:nginx"},
			pauseCtn: true,
			container: filterdef.Container{
				Name:  "nginx",
				Image: "kubernetes/pause",
			},
			defaultValue: false,
			expected:     true,
		},
		{
			name:     "Include pause container",
			include:  []string{""},
			exclude:  []string{""},
			pauseCtn: false,
			container: filterdef.Container{
				Image: "kubernetes/pause",
			},
			defaultValue: false,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("sbom.container_image.container_include", tt.include)
			mockConfig.SetWithoutSource("sbom.container_image.container_exclude", tt.exclude)
			mockConfig.SetWithoutSource("sbom.container_image.exclude_pause_container", tt.pauseCtn)

			f := newFilterObject(t, mockConfig)

			res, err := f.IsContainerExcluded(tt.container, []filterdef.ContainerFilter{filterdef.ContainerSBOM}, tt.defaultValue)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, res, "Container exclusion result mismatch")
		})
	}

}
