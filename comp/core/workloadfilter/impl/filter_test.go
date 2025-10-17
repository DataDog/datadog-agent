// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel && test

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Create a new filter object for testing purposes
func newFilterStoreObject(t *testing.T, config config.Component) *workloadfilterStore {
	reqs := Requires{
		Log:    logmock.New(t),
		Config: config,
	}
	f, err := NewComponent(reqs)
	if err != nil {
		t.Errorf("failed to create filter component: %v", err)
		return nil
	}
	return f.Comp.(*workloadfilterStore)
}

func TestBasicFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"image:datadog/agent:latest"})
	filterStore := newFilterStoreObject(t, mockConfig)

	t.Run("empty filters, empty container", func(t *testing.T) {
		container := &workloadfilter.Container{}
		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Unknown, res)
	})

	t.Run("single include filter", func(t *testing.T) {
		container := workloadmetafilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			nil,
		)

		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Included, res)
	})

	t.Run("single exclude filter", func(t *testing.T) {
		container := workloadmetafilter.CreateContainer(
			&workloadmeta.Container{
				Image: workloadmeta.ContainerImage{
					RawName: "datadog/agent:latest",
				},
			},
			nil,
		)

		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("include beats exclude", func(t *testing.T) {
		container := workloadmetafilter.CreateContainer(
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

		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Included, res)
	})

}

func TestADAnnotationFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	filterStore := newFilterStoreObject(t, mockConfig)

	t.Run("improper exclude annotation", func(t *testing.T) {
		pod := workloadmetafilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/garbage": "true",
					},
				},
			},
		)
		container := workloadmetafilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			pod,
		)

		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotations}})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Unknown, res)
	})

	t.Run("proper exclude annotation", func(t *testing.T) {
		pod := workloadmetafilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/dd-agent.exclude": "true",
					},
				},
			},
		)
		container := workloadmetafilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			pod,
		)

		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotations}})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("blank container name", func(t *testing.T) {
		// Edge case if the container name is missing

		pod := workloadmetafilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/.exclude": "true",
					},
				},
			},
		)

		container := workloadmetafilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "some-container",
				},
			},
			pod,
		)
		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotations}})
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Unknown, res)
	})

}

func TestCombinedFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"name:nginx"})
	mockConfig.SetWithoutSource("ac_include", []string{"kube_namespace:default"})
	mockConfig.SetWithoutSource("ac_exclude", []string{"kube_namespace:datadog-agent"})

	filterStore := newFilterStoreObject(t, mockConfig)

	container := workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
		},
		nil,
	)

	filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
	res := filterBundle.IsExcluded(container)
	assert.Equal(t, false, res)

	pod := workloadmetafilter.CreatePod(
		&workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "default",
			},
		},
	)
	container = workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
		},
		pod,
	)

	filterBundle = filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerACInclude}})
	res = filterBundle.IsExcluded(container)
	assert.Equal(t, false, res)

	container = workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "nginx",
			},
		},
		pod,
	)

	filterBundle = filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerACExclude, workloadfilter.LegacyContainerACInclude}})
	res = filterBundle.IsExcluded(container)
	assert.Equal(t, false, res)

	pod = workloadmetafilter.CreatePod(
		&workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "datadog-agent",
			},
		},
	)
	container = workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "nginx",
			},
		},
		pod,
	)
	filterBundle = filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerACExclude, workloadfilter.LegacyContainerACInclude}})
	res = filterBundle.IsExcluded(container)
	assert.Equal(t, true, res)
}

func TestContainerSBOMFilter(t *testing.T) {

	tests := []struct {
		name      string
		include   []string
		exclude   []string
		pauseCtn  bool
		container *workloadfilter.Container
		expected  bool
	}{
		{
			name:     "Include image",
			include:  []string{"image:dd-agent"},
			exclude:  []string{"image:nginx"},
			pauseCtn: false,
			container: workloadmetafilter.CreateContainer(
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
			container: workloadmetafilter.CreateContainer(
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
			container: workloadmetafilter.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
				},
				workloadmetafilter.CreatePod(
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
			container: workloadmetafilter.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
				},
				workloadmetafilter.CreatePod(
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
			container: workloadmetafilter.CreateContainer(
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
			container: workloadmetafilter.CreateContainer(
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
			mockConfig.SetWithoutSource("sbom.container_image.enabled", true)
			mockConfig.SetWithoutSource("sbom.container_image.container_include", tt.include)
			mockConfig.SetWithoutSource("sbom.container_image.container_exclude", tt.exclude)
			mockConfig.SetWithoutSource("sbom.container_image.exclude_pause_container", tt.pauseCtn)

			filterStore := newFilterStoreObject(t, mockConfig)
			filterBundle := filterStore.GetContainerSBOMFilters()
			res := filterBundle.IsExcluded(tt.container)
			assert.Equal(t, tt.expected, res, "Container exclusion result mismatch")
		})
	}
}

func TestFilterPrecedence(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_exclude", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_include_metrics", []string{"name:dd-agent"})

	filterStore := newFilterStoreObject(t, mockConfig)

	container := workloadmetafilter.CreateContainer(
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
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerGlobal},  // Excludes (higher priority)
			{workloadfilter.LegacyContainerMetrics}, // Includes (but lower priority)
		}
		filterBundle := filterStore.GetContainerFilters(precedenceFilters)

		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("First set includes, second set not evaluated", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerMetrics}, // Includes (higher priority)
			{workloadfilter.LegacyContainerGlobal},  // Excludes (but lower priority)
		}
		filterBundle := filterStore.GetContainerFilters(precedenceFilters)

		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Included, res)
	})

	t.Run("First set unknown, second set exclude", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerLogs},   // Unknown, no results
			{workloadfilter.LegacyContainerGlobal}, // Excludes
		}
		filterBundle := filterStore.GetContainerFilters(precedenceFilters)

		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("First set unknown, second set include", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerLogs},    // Unknown, no results
			{workloadfilter.LegacyContainerMetrics}, // Includes
		}
		filterBundle := filterStore.GetContainerFilters(precedenceFilters)

		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Included, res)
	})
}

func TestEvaluateResourceNoFilters(t *testing.T) {
	mockConfig := configmock.New(t)
	filterStore := newFilterStoreObject(t, mockConfig)

	container := workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "no-filter",
			},
		},
		nil,
	)

	t.Run("No filter sets", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{}
		filterBundle := filterStore.GetContainerFilters(precedenceFilters)
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Unknown, res)
	})

	t.Run("Empty filter set", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{}, {}, {}, {}, {}, {}, {}, {}, {},
		}
		filterBundle := filterStore.GetContainerFilters(precedenceFilters)
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Unknown, res)
	})
}

func TestContainerFilterInitializationError(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"bad_name:nginx"})
	mockConfig.SetWithoutSource("container_include_metrics", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("ac_include", []string{"other_bad_name:nginx"})
	filterStore := newFilterStoreObject(t, mockConfig)

	t.Run("Properly defined filter", func(t *testing.T) {
		errs := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerMetrics}}).GetErrors()
		assert.Empty(t, errs, "Expected no initialization errors for properly defined filter")
	})

	t.Run("Improperly defined filter", func(t *testing.T) {
		errs := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}}).GetErrors()
		assert.NotEmpty(t, errs, "Expected initialization errors for improperly defined filter")
		assert.True(t, containsErrorWithMessage(errs, "bad_name"), "Expected error message to contain the improper key 'bad_name'")
	})

	t.Run("Improperly defined filter with multiple filters", func(t *testing.T) {
		errs := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotationsMetrics}, {workloadfilter.LegacyContainerMetrics}, {workloadfilter.LegacyContainerACInclude}}).GetErrors()
		assert.NotEmpty(t, errs, "Expected initialization errors for improperly defined filter with multiple filters")
		assert.True(t, containsErrorWithMessage(errs, "other_bad_name"), "Expected error message to contain the improper key 'other_bad_name'")
		assert.True(t, containsErrorWithMessage(errs, "bad_name"), "Expected error message to contain the improper key 'bad_name'")
	})
}

type errorInclProgram struct{}

func (p errorInclProgram) Evaluate(o workloadfilter.Filterable) (workloadfilter.Result, []error) {
	return workloadfilter.Included, []error{fmt.Errorf("include evaluation error on %s", o.Type())}
}

func (p errorInclProgram) GetInitializationErrors() []error {
	return nil
}

type errorExclProgram struct{}

func (p errorExclProgram) Evaluate(o workloadfilter.Filterable) (workloadfilter.Result, []error) {
	return workloadfilter.Excluded, []error{fmt.Errorf("exclude evaluation error on %s", o.Type())}
}

func (p errorExclProgram) GetInitializationErrors() []error {
	return nil
}

func TestProgramErrorHandling(t *testing.T) {
	mockConfig := configmock.New(t)
	filterStore := newFilterStoreObject(t, mockConfig)

	container := workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "error-case",
			},
		},
		nil,
	)
	precedenceFilters := [][]workloadfilter.ContainerFilter{
		{workloadfilter.LegacyContainerMetrics},
	}

	t.Run("Include with error thrown", func(t *testing.T) {
		// Create a new filter with injected error factory
		errorFilter := &workloadfilterStore{
			config:              filterStore.config,
			log:                 filterStore.log,
			telemetry:           filterStore.telemetry,
			programFactoryStore: make(map[workloadfilter.ResourceType]map[int]*filterProgramFactory),
		}

		// Register the error program factory
		errorFilter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerMetrics),
			func(_ *catalog.FilterConfig, _ log.Component) program.FilterProgram {
				return &errorInclProgram{}
			})

		filterBundle := errorFilter.GetContainerFilters(precedenceFilters)
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Included, res)
	})

	t.Run("Exclude with error thrown", func(t *testing.T) {
		// Create a new filter with injected error factory
		errorFilter := &workloadfilterStore{
			config:              filterStore.config,
			log:                 filterStore.log,
			telemetry:           filterStore.telemetry,
			programFactoryStore: make(map[workloadfilter.ResourceType]map[int]*filterProgramFactory),
		}

		// Register the error program factory
		errorFilter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerMetrics),
			func(_ *catalog.FilterConfig, _ log.Component) program.FilterProgram {
				return &errorExclProgram{}
			})

		filterBundle := errorFilter.GetContainerFilters(precedenceFilters)
		res := filterBundle.GetResult(container)
		assert.Equal(t, workloadfilter.Excluded, res)
	})
}

func TestSpecialCharacters(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{`name:g'oba\\r\d-0x[0-9a-fA-F]+\\n`})
	filterStore := newFilterStoreObject(t, mockConfig)

	container := workloadmetafilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: `g'oba\r9-0xDEADBEEF\n`,
			},
		},
		nil,
	)

	precedenceFilters := [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}}
	filterBundle := filterStore.GetContainerFilters(precedenceFilters)
	res := filterBundle.GetResult(container)
	assert.Equal(t, workloadfilter.Included, res)
}

func TestServiceFiltering(t *testing.T) {
	tests := []struct {
		name        string
		include     []string
		exclude     []string
		serviceName string
		namespace   string
		annotations map[string]string
		filters     [][]workloadfilter.ServiceFilter
		expected    workloadfilter.Result
	}{
		{
			name:        "Exclude by name",
			exclude:     []string{"name:svc1"},
			serviceName: "svc1",
			namespace:   "",
			annotations: nil,
			filters:     [][]workloadfilter.ServiceFilter{{workloadfilter.LegacyServiceGlobal}},
			expected:    workloadfilter.Excluded,
		},
		{
			name:        "Exclude by namespace",
			exclude:     []string{"kube_namespace:test"},
			serviceName: "my-service",
			namespace:   "test",
			annotations: nil,
			filters:     [][]workloadfilter.ServiceFilter{{workloadfilter.LegacyServiceGlobal}},
			expected:    workloadfilter.Excluded,
		},
		{
			name:        "AD annotation exclude",
			serviceName: "annotated-service",
			namespace:   "default",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "true",
			},
			filters:  [][]workloadfilter.ServiceFilter{{workloadfilter.ServiceADAnnotations}},
			expected: workloadfilter.Excluded,
		},
		{
			name:        "AD annotation metrics exclude",
			serviceName: "metrics-excluded-service",
			namespace:   "default",
			annotations: map[string]string{
				"ad.datadoghq.com/metrics_exclude": "true",
			},
			filters:  [][]workloadfilter.ServiceFilter{{workloadfilter.ServiceADAnnotationsMetrics}},
			expected: workloadfilter.Excluded,
		},
		{
			name:        "AD annotation exclude truthy values",
			serviceName: "annotated-service",
			namespace:   "default",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "T",
			},
			filters:  [][]workloadfilter.ServiceFilter{{workloadfilter.ServiceADAnnotations}},
			expected: workloadfilter.Excluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			if len(tt.exclude) > 0 {
				mockConfig.SetWithoutSource("container_exclude", tt.exclude)
			}
			filterStore := newFilterStoreObject(t, mockConfig)
			filterBundle := filterStore.GetServiceFilters(tt.filters)

			service := workloadfilter.CreateService(tt.serviceName, tt.namespace, tt.annotations)

			res := filterBundle.GetResult(service)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestEndpointFiltering(t *testing.T) {
	tests := []struct {
		name         string
		include      []string
		exclude      []string
		endpointName string
		namespace    string
		annotations  map[string]string
		filters      [][]workloadfilter.EndpointFilter
		expected     workloadfilter.Result
	}{
		{
			name:         "Exclude by name",
			exclude:      []string{"name:ep1"},
			endpointName: "ep1",
			namespace:    "",
			annotations:  nil,
			filters:      [][]workloadfilter.EndpointFilter{{workloadfilter.LegacyEndpointGlobal}},
			expected:     workloadfilter.Excluded,
		},
		{
			name:         "Exclude by namespace",
			exclude:      []string{"kube_namespace:test"},
			endpointName: "my-endpoint",
			namespace:    "test",
			annotations:  nil,
			filters:      [][]workloadfilter.EndpointFilter{{workloadfilter.LegacyEndpointGlobal}},
			expected:     workloadfilter.Excluded,
		},
		{
			name:         "AD annotation exclude",
			endpointName: "annotated-endpoint",
			namespace:    "default",
			annotations: map[string]string{
				"ad.datadoghq.com/exclude": "true",
			},
			filters:  [][]workloadfilter.EndpointFilter{{workloadfilter.EndpointADAnnotations}},
			expected: workloadfilter.Excluded,
		},
		{
			name:         "AD annotation metrics exclude",
			endpointName: "metrics-excluded-endpoint",
			namespace:    "default",
			annotations: map[string]string{
				"ad.datadoghq.com/metrics_exclude": "true",
			},
			filters:  [][]workloadfilter.EndpointFilter{{workloadfilter.EndpointADAnnotationsMetrics}},
			expected: workloadfilter.Excluded,
		},
		{
			name:         "AD annotation exclude truthy values",
			endpointName: "metrics-excluded-endpoint",
			namespace:    "default",
			annotations: map[string]string{
				"ad.datadoghq.com/metrics_exclude": "1",
			},
			filters:  [][]workloadfilter.EndpointFilter{{workloadfilter.EndpointADAnnotationsMetrics}},
			expected: workloadfilter.Excluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			if len(tt.exclude) > 0 {
				mockConfig.SetWithoutSource("container_exclude", tt.exclude)
			}
			filterStore := newFilterStoreObject(t, mockConfig)
			filterBundle := filterStore.GetEndpointFilters(tt.filters)

			endpoint := workloadfilter.CreateEndpoint(tt.endpointName, tt.namespace, tt.annotations)

			res := filterBundle.GetResult(endpoint)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestImageFiltering(t *testing.T) {
	tests := []struct {
		name      string
		include   []string
		exclude   []string
		imageName string
		expected  workloadfilter.Result
	}{
		{
			name:      "Include by image",
			include:   []string{"image:dd-agent"},
			exclude:   []string{"image:nginx"},
			imageName: "dd-agent",
			expected:  workloadfilter.Included,
		},
		{
			name:      "Exclude by image",
			include:   []string{"image:dd-agent"},
			exclude:   []string{"image:nginx"},
			imageName: "nginx-123",
			expected:  workloadfilter.Excluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("container_include", tt.include)
			mockConfig.SetWithoutSource("container_exclude", tt.exclude)
			filterStore := newFilterStoreObject(t, mockConfig)

			containerFilters := filterStore.GetContainerSharedMetricFilters()
			containerImage := workloadfilter.CreateContainerImage(tt.imageName)

			res := containerFilters.GetResult(containerImage)
			assert.Equal(t, tt.expected, res)
		})
	}
}

// containsErrorWithMessage checks if any error in the slice contains the specified message
func containsErrorWithMessage(errs []error, message string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), message) {
			return true
		}
	}
	return false
}

func TestPodFiltering(t *testing.T) {
	tests := []struct {
		name     string
		include  []string
		exclude  []string
		wmetaPod *workloadmeta.KubernetesPod
		filters  [][]workloadfilter.PodFilter
		expected workloadfilter.Result
	}{
		{
			name:    "Exclude by namespace",
			exclude: []string{"kube_namespace:default"},
			wmetaPod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "pod1",
					Namespace: "default",
				},
			},
			filters:  [][]workloadfilter.PodFilter{{workloadfilter.LegacyPod}},
			expected: workloadfilter.Excluded,
		},
		{
			name:    "Include by namespace",
			include: []string{"kube_namespace:test"},
			wmetaPod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "my-pod",
					Namespace: "test",
				},
			},
			filters:  [][]workloadfilter.PodFilter{{workloadfilter.LegacyPod}},
			expected: workloadfilter.Included,
		},
		{
			name: "AD annotation exclude",
			wmetaPod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "annotated-pod",
					Annotations: map[string]string{
						"ad.datadoghq.com/exclude": "true",
					},
				},
			},
			// Testing PodADAnnotations filter
			filters:  [][]workloadfilter.PodFilter{{workloadfilter.PodADAnnotations, workloadfilter.PodADAnnotationsMetrics}, {workloadfilter.LegacyPod}},
			expected: workloadfilter.Excluded,
		},
		{
			name: "AD annotation metrics exclude",
			wmetaPod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "metrics-excluded-pod",
					Annotations: map[string]string{
						"ad.datadoghq.com/metrics_exclude": "true",
					},
				},
			},
			// Testing PodADAnnotationsMetrics filter
			filters:  [][]workloadfilter.PodFilter{{workloadfilter.PodADAnnotations, workloadfilter.PodADAnnotationsMetrics}, {workloadfilter.LegacyPod}},
			expected: workloadfilter.Excluded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("container_include", tt.include)
			mockConfig.SetWithoutSource("container_exclude", tt.exclude)
			filterStore := newFilterStoreObject(t, mockConfig)
			filterBundle := filterStore.GetPodFilters(tt.filters)

			pod := workloadmetafilter.CreatePod(tt.wmetaPod)

			res := filterBundle.GetResult(pod)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestProcessFiltering(t *testing.T) {
	tests := []struct {
		name             string
		disallowPatterns []string
		comm             string
		cmdline          []string
		filters          [][]workloadfilter.ProcessFilter
		expected         workloadfilter.Result
	}{
		{
			name:             "empty filters, empty process",
			disallowPatterns: []string{},
			filters:          [][]workloadfilter.ProcessFilter{},
			expected:         workloadfilter.Unknown,
		},
		{
			name:             "process excluded by cmdline pattern",
			disallowPatterns: []string{"java.*", "systemd", "/usr/bin/.*"},
			cmdline:          []string{"java", "-server", "-Xmx2g"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Excluded,
		},
		{
			name:             "process excluded by systemd pattern in cmdline",
			disallowPatterns: []string{"java.*", "systemd", "/usr/bin/.*"},
			cmdline:          []string{"systemd", "--user"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Excluded,
		},
		{
			name:             "process excluded by /usr/bin pattern in cmdline",
			disallowPatterns: []string{"java.*", "systemd", "/usr/bin/.*"},
			cmdline:          []string{"/usr/bin/python3", "script.py"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Excluded,
		},
		{
			name:             "process not excluded",
			disallowPatterns: []string{"java.*", "systemd", "/usr/bin/.*"},
			cmdline:          []string{"nginx", "-g", "daemon off;"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Unknown,
		},
		{
			name:             "pattern spanning multiple arguments - python script",
			disallowPatterns: []string{"python.*script", "java.*-jar.*app", "node.*server"},
			cmdline:          []string{"python3", "manage.py", "runserver", "script.py"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Excluded,
		},
		{
			name:             "pattern spanning multiple arguments - java jar app",
			disallowPatterns: []string{"python.*script", "java.*-jar.*app", "node.*server"},
			cmdline:          []string{"java", "-Xmx2g", "-jar", "myapp.jar", "--port", "8080"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Excluded,
		},
		{
			name:             "no patterns match",
			disallowPatterns: []string{"python.*script", "java.*-jar.*app", "node.*server"},
			cmdline:          []string{"nginx", "-g", "daemon off;"},
			filters:          [][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}},
			expected:         workloadfilter.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			if len(tt.disallowPatterns) > 0 {
				mockConfig.SetWithoutSource("process_config.blacklist_patterns", tt.disallowPatterns)
			}
			f := newFilterStoreObject(t, mockConfig)

			var process *workloadfilter.Process
			if tt.name == "empty filters, empty process" {
				process = &workloadfilter.Process{}
			} else {
				process = workloadmetafilter.CreateProcess(
					&workloadmeta.Process{
						Comm:    tt.comm,
						Cmdline: tt.cmdline,
					},
				)
			}

			filterBundle := f.GetProcessFilters(tt.filters)
			res := filterBundle.GetResult(process)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestProcessFilterInitializationError(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("process_config.blacklist_patterns", []string{
		"valid_pattern",
		"[invalid_regex",
	})
	f := newFilterStoreObject(t, mockConfig)

	t.Run("Invalid regex patterns cause initialization errors", func(t *testing.T) {
		filters := f.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.LegacyProcessExcludeList}})
		errs := filters.GetErrors()
		assert.NotEmpty(t, errs, "Expected initialization errors for invalid regex patterns")

		errStrings := make([]string, len(errs))
		for i, err := range errs {
			errStrings[i] = err.Error()
		}

		hasRegexError := false
		for _, errStr := range errStrings {
			if strings.Contains(errStr, "invalid_regex") || strings.Contains(errStr, "error parsing regexp") {
				hasRegexError = true
				break
			}
		}
		assert.True(t, hasRegexError, "Expected error message to contain regex-related error. Got errors: %v", errStrings)
	})
}
func TestCELWorkloadExcludeFiltering(t *testing.T) {

	yamlConfig := `
cel_workload_exclude:
- products: ["metrics"]
  rules:
    kube_services: ["true"]
    pods: ["false"]
- products:
    - logs
    - sbom
  rules:
    containers:
      - "container.name != 'this'"
`

	mockConfig := configmock.NewFromYAML(t, yamlConfig)
	filterStore := newFilterStoreObject(t, mockConfig)

	t.Run("CEL exclude kube_service", func(t *testing.T) {
		svc := workloadfilter.CreateService("", "", nil)
		filterBundle := filterStore.GetServiceFilters([][]workloadfilter.ServiceFilter{{workloadfilter.ServiceFilter(workloadfilter.ServiceCELMetrics)}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, workloadfilter.Excluded, filterBundle.GetResult(svc))
	})

	t.Run("CEL exclude pod", func(t *testing.T) {
		pod := workloadmetafilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "my-pod",
					Namespace: "test",
				},
			},
		)
		filterBundle := filterStore.GetPodFilters([][]workloadfilter.PodFilter{{workloadfilter.PodFilter(workloadfilter.PodCELMetrics)}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, false, filterBundle.IsExcluded(pod))
	})

	t.Run("CEL exclude container", func(t *testing.T) {
		container := workloadmetafilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "this",
				},
			},
			nil,
		)
		filterBundle := filterStore.GetContainerFilters([][]workloadfilter.ContainerFilter{{workloadfilter.ContainerFilter(workloadfilter.ContainerCELLogs)}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, false, filterBundle.IsExcluded(container))
	})
}

func TestCELProcessLogsFiltering(t *testing.T) {
	yamlConfig := `
cel_workload_exclude:
- products: ["logs"]
  rules:
    processes:
      - "process.name == 'nginx'"
      - "process.cmdline.contains('-jar app.jar')"
      - "process.args.exists(arg, arg == 'ignore-me')"
`

	mockConfig := configmock.NewFromYAML(t, yamlConfig)
	filterStore := newFilterStoreObject(t, mockConfig)

	t.Run("CEL exclude process by name", func(t *testing.T) {
		process := workloadfilter.CreateProcess("nginx", []string{"nginx", "-g", "daemon off;"})
		filterBundle := filterStore.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.ProcessCELLogs}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, workloadfilter.Excluded, filterBundle.GetResult(process))
	})

	t.Run("CEL exclude process by cmdline", func(t *testing.T) {
		process := workloadfilter.CreateProcess("java", []string{"java", "-jar", "app.jar"})
		filterBundle := filterStore.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.ProcessCELLogs}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, workloadfilter.Excluded, filterBundle.GetResult(process))
	})

	t.Run("CEL exclude process by args", func(t *testing.T) {
		process := workloadfilter.CreateProcess("foobar", []string{"ignore-me", "--foo", "bar"})
		filterBundle := filterStore.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.ProcessCELLogs}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, workloadfilter.Excluded, filterBundle.GetResult(process))
	})

	t.Run("CEL with no matching rule", func(t *testing.T) {
		process := workloadfilter.CreateProcess("redis-server", []string{"/usr/bin/redis-server"})
		filterBundle := filterStore.GetProcessFilters([][]workloadfilter.ProcessFilter{{workloadfilter.ProcessCELLogs}})
		assert.Nil(t, filterBundle.GetErrors())
		assert.Equal(t, workloadfilter.Unknown, filterBundle.GetResult(process))
	})
}
