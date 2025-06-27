// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterimpl contains the implementation of the filter component.
package filterimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	filterdef "github.com/DataDog/datadog-agent/comp/core/filter/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

func TestBasicFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"image:datadog/agent:latest"})
	f := newFilterObject(t, mockConfig)

	t.Run("empty filters, empty container", func(t *testing.T) {
		container := &filterdef.Container{}
		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{})
		assert.Equal(t, filterdef.Unknown, res)
	})

	t.Run("single include filter", func(t *testing.T) {
		container := filterdef.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			nil,
		)

		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal}})
		assert.Equal(t, filterdef.Included, res)
	})

	t.Run("single exclude filter", func(t *testing.T) {
		container := filterdef.CreateContainer(
			&workloadmeta.Container{
				Image: workloadmeta.ContainerImage{
					RawName: "datadog/agent:latest",
				},
			},
			nil,
		)

		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal}})
		assert.Equal(t, filterdef.Excluded, res)
	})

	t.Run("include beats exclude", func(t *testing.T) {
		container := filterdef.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
				Image: workloadmeta.ContainerImage{
					RawName: "datadog/agent:latest",
				},
			},
			nil,
		)
		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal}})
		assert.Equal(t, filterdef.Included, res)
	})

}

func TestADAnnotationFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	f := newFilterObject(t, mockConfig)

	t.Run("improper exclude annotation", func(t *testing.T) {
		pod := filterdef.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/garbage": "true",
					},
				},
			},
		)
		container := filterdef.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			pod,
		)

		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.ContainerADAnnotations}})
		assert.Equal(t, filterdef.Unknown, res)
	})

	t.Run("proper exclude annotation", func(t *testing.T) {
		pod := filterdef.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/dd-agent.exclude": "true",
					},
				},
			},
		)
		container := filterdef.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			pod,
		)

		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.ContainerADAnnotations}})
		assert.Equal(t, filterdef.Excluded, res)
	})

	t.Run("blank container name", func(t *testing.T) {
		// Edge case if the container name is missing

		pod := filterdef.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/.exclude": "true",
					},
				},
			},
		)

		container := filterdef.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "some-container",
				},
			},
			pod,
		)
		res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.ContainerADAnnotations}})
		assert.Equal(t, filterdef.Unknown, res)
	})

}

func TestCombinedFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"name:nginx"})
	mockConfig.SetWithoutSource("ac_include", []string{"kube_namespace:default"})
	mockConfig.SetWithoutSource("ac_exclude", []string{"kube_namespace:datadog-agent"})

	f := newFilterObject(t, mockConfig)

	container := filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
		},
		nil,
	)

	res := f.IsContainerExcluded(container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal}})
	assert.Equal(t, false, res)

	pod := filterdef.CreatePod(
		&workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "default",
			},
		},
	)
	container = filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
		},
		pod,
	)

	res = f.IsContainerExcluded(container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal, filterdef.LegacyContainerACExclude, filterdef.LegacyContainerACInclude}})
	assert.Equal(t, false, res)

	container = filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "nginx",
			},
		},
		pod,
	)
	res = f.IsContainerExcluded(container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal, filterdef.LegacyContainerACExclude, filterdef.LegacyContainerACInclude}})
	assert.Equal(t, false, res)

	pod = filterdef.CreatePod(
		&workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "datadog-agent",
			},
		},
	)
	container = filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "nginx",
			},
		},
		pod,
	)
	res = f.IsContainerExcluded(container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal, filterdef.LegacyContainerACExclude, filterdef.LegacyContainerACInclude}})
	assert.Equal(t, true, res)
}

func TestContainerSBOMFilter(t *testing.T) {

	tests := []struct {
		name      string
		include   []string
		exclude   []string
		pauseCtn  bool
		container *filterdef.Container
		expected  bool
	}{
		{
			name:     "Include image",
			include:  []string{"image:dd-agent"},
			exclude:  []string{"image:nginx"},
			pauseCtn: false,
			container: filterdef.CreateContainer(
				&workloadmeta.Container{
					Image: workloadmeta.ContainerImage{
						RawName: "dd-agent",
					},
				},
				nil,
			),
			expected: false,
		},
		{
			name:     "Exclude image",
			include:  []string{"image:dd-agent"},
			exclude:  []string{"image:nginx"},
			pauseCtn: false,
			container: filterdef.CreateContainer(
				&workloadmeta.Container{
					Image: workloadmeta.ContainerImage{
						RawName: "nginx-123",
					},
				},
				nil,
			),
			expected: true,
		},
		{
			name:     "Included namespace beats excluded name",
			include:  []string{"kube_namespace:default"},
			exclude:  []string{"name:nginx"},
			pauseCtn: false,
			container: filterdef.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
				},
				filterdef.CreatePod(
					&workloadmeta.KubernetesPod{
						EntityMeta: workloadmeta.EntityMeta{
							Namespace: "default",
						},
					},
				),
			),
			expected: false,
		},
		{
			name:     "Included name beats excluded namespace",
			include:  []string{"name:nginx"},
			exclude:  []string{"kube_namespace:default"},
			pauseCtn: false,
			container: filterdef.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
				},
				filterdef.CreatePod(
					&workloadmeta.KubernetesPod{
						EntityMeta: workloadmeta.EntityMeta{
							Namespace: "default",
						},
					},
				),
			),
			expected: false,
		},
		{
			name:     "Exclude pause container",
			include:  []string{""},
			exclude:  []string{""},
			pauseCtn: true,
			container: filterdef.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
					Image: workloadmeta.ContainerImage{
						RawName: "kubernetes/pause",
					},
				},
				nil,
			),
			expected: true,
		},
		{
			name:     "Include pause container",
			include:  []string{""},
			exclude:  []string{""},
			pauseCtn: false,
			container: filterdef.CreateContainer(
				&workloadmeta.Container{
					Image: workloadmeta.ContainerImage{
						RawName: "kubernetes/pause",
					},
				},
				nil,
			),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("sbom.container_image.container_include", tt.include)
			mockConfig.SetWithoutSource("sbom.container_image.container_exclude", tt.exclude)
			mockConfig.SetWithoutSource("sbom.container_image.exclude_pause_container", tt.pauseCtn)

			f := newFilterObject(t, mockConfig)

			res := f.IsContainerExcluded(tt.container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerSBOM}})
			assert.Equal(t, tt.expected, res, "Container exclusion result mismatch")
		})
	}
}

func TestFilterPrecedence(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_exclude", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_include_metrics", []string{"name:dd-agent"})

	f := newFilterObject(t, mockConfig)

	container := filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
			Image: workloadmeta.ContainerImage{
				RawName: "datadog/agent:latest",
			},
		},
		nil,
	)

	t.Run("First set excludes, second set not evaluated", func(t *testing.T) {
		precedenceFilters := [][]filterdef.ContainerFilter{
			{filterdef.LegacyContainerGlobal},  // Excludes (higher priority)
			{filterdef.LegacyContainerMetrics}, // Includes (but lower priority)
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Excluded, res)
	})

	t.Run("First set includes, second set not evaluated", func(t *testing.T) {
		precedenceFilters := [][]filterdef.ContainerFilter{
			{filterdef.LegacyContainerMetrics}, // Includes (higher priority)
			{filterdef.LegacyContainerGlobal},  // Excludes (but lower priority)
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Included, res)
	})

	t.Run("First set unknown, second set exclude", func(t *testing.T) {
		precedenceFilters := [][]filterdef.ContainerFilter{
			{filterdef.LegacyContainerLogs},   // Unknown, no results
			{filterdef.LegacyContainerGlobal}, // Excludes
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Excluded, res)
	})

	t.Run("First set unknown, second set include", func(t *testing.T) {
		precedenceFilters := [][]filterdef.ContainerFilter{
			{filterdef.LegacyContainerLogs},    // Unknown, no results
			{filterdef.LegacyContainerMetrics}, // Includes
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Included, res)
	})
}

func TestEvaluateResourceNoFilters(t *testing.T) {
	mockConfig := configmock.New(t)
	f := newFilterObject(t, mockConfig)

	container := filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "no-filter",
			},
		},
		nil,
	)

	t.Run("No filter sets", func(t *testing.T) {
		precedenceFilters := [][]filterdef.ContainerFilter{}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Unknown, res)
	})

	t.Run("Empty filter set", func(t *testing.T) {
		precedenceFilters := [][]filterdef.ContainerFilter{
			{}, {}, {}, {}, {}, {}, {}, {}, {},
		}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Unknown, res)
	})
}

type errorInclProgram struct{}

func (p errorInclProgram) Evaluate(o filterdef.Filterable) (filterdef.Result, []error) {
	return filterdef.Included, []error{fmt.Errorf("include evaluation error on %s", o.Type())}
}

type errorExclProgram struct{}

func (p errorExclProgram) Evaluate(o filterdef.Filterable) (filterdef.Result, []error) {
	return filterdef.Excluded, []error{fmt.Errorf("exclude evaluation error on %s", o.Type())}
}

func TestProgramErrorHandling(t *testing.T) {
	mockConfig := configmock.New(t)
	f := newFilterObject(t, mockConfig)

	container := filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "error-case",
			},
		},
		nil,
	)
	precedenceFilters := [][]filterdef.ContainerFilter{
		{filterdef.LegacyContainerMetrics},
	}

	t.Run("Include with error thrown", func(t *testing.T) {
		// Inject a program that always errors for ContainerMetrics, but returns Included
		f.prgs[filterdef.ContainerType][int(filterdef.LegacyContainerMetrics)] = &errorInclProgram{}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Included, res)
	})

	t.Run("Exclude with error thrown", func(t *testing.T) {
		// Inject a program that always errors for ContainerMetrics, but returns Excluded
		f.prgs[filterdef.ContainerType][int(filterdef.LegacyContainerMetrics)] = &errorExclProgram{}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, filterdef.Excluded, res)
	})
}

func TestSpecialCharacters(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{`name:g'oba\\r\d-0x[0-9a-fA-F]+\\n`})
	f := newFilterObject(t, mockConfig)

	container := filterdef.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: `g'oba\r9-0xDEADBEEF\n`,
			},
		},
		nil,
	)

	res := evaluateResource(f, container, [][]filterdef.ContainerFilter{{filterdef.LegacyContainerGlobal}})
	assert.Equal(t, filterdef.Included, res)
}

func TestServiceFiltering(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{`name:svc1`})
	f := newFilterObject(t, mockConfig)

	service := filterdef.CreateService("svc1", "", nil)

	res := evaluateResource(f, service, [][]filterdef.ServiceFilter{{filterdef.LegacyServiceGlobal}})
	assert.Equal(t, filterdef.Included, res)
}

func TestEndpointFiltering(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{`name:ep1`})
	f := newFilterObject(t, mockConfig)

	endpoint := filterdef.CreateEndpoint("ep1", "", nil)

	res := evaluateResource(f, endpoint, [][]filterdef.EndpointFilter{{filterdef.LegacyEndpointGlobal}})
	assert.Equal(t, filterdef.Included, res)
}
