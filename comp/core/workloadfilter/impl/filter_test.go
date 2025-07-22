// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
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
		container := &workloadfilter.Container{}
		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{})
		assert.Equal(t, workloadfilter.Unknown, res)
	})

	t.Run("single include filter", func(t *testing.T) {
		container := workloadfilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			nil,
		)

		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
		assert.Equal(t, workloadfilter.Included, res)
	})

	t.Run("single exclude filter", func(t *testing.T) {
		container := workloadfilter.CreateContainer(
			&workloadmeta.Container{
				Image: workloadmeta.ContainerImage{
					RawName: "datadog/agent:latest",
				},
			},
			nil,
		)

		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("include beats exclude", func(t *testing.T) {
		container := workloadfilter.CreateContainer(
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
		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
		assert.Equal(t, workloadfilter.Included, res)
	})

}

func TestADAnnotationFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	f := newFilterObject(t, mockConfig)

	t.Run("improper exclude annotation", func(t *testing.T) {
		pod := workloadfilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/garbage": "true",
					},
				},
			},
		)
		container := workloadfilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			pod,
		)

		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotations}})
		assert.Equal(t, workloadfilter.Unknown, res)
	})

	t.Run("proper exclude annotation", func(t *testing.T) {
		pod := workloadfilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/dd-agent.exclude": "true",
					},
				},
			},
		)
		container := workloadfilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "dd-agent",
				},
			},
			pod,
		)

		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotations}})
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("blank container name", func(t *testing.T) {
		// Edge case if the container name is missing

		pod := workloadfilter.CreatePod(
			&workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/.exclude": "true",
					},
				},
			},
		)

		container := workloadfilter.CreateContainer(
			&workloadmeta.Container{
				EntityMeta: workloadmeta.EntityMeta{
					Name: "some-container",
				},
			},
			pod,
		)
		res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.ContainerADAnnotations}})
		assert.Equal(t, workloadfilter.Unknown, res)
	})

}

func TestCombinedFilter(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"name:nginx"})
	mockConfig.SetWithoutSource("ac_include", []string{"kube_namespace:default"})
	mockConfig.SetWithoutSource("ac_exclude", []string{"kube_namespace:datadog-agent"})

	f := newFilterObject(t, mockConfig)

	container := workloadfilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
		},
		nil,
	)

	res := f.IsContainerExcluded(container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
	assert.Equal(t, false, res)

	pod := workloadfilter.CreatePod(
		&workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "default",
			},
		},
	)
	container = workloadfilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "dd-agent",
			},
		},
		pod,
	)

	res = f.IsContainerExcluded(container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerACExclude, workloadfilter.LegacyContainerACInclude}})
	assert.Equal(t, false, res)

	container = workloadfilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "nginx",
			},
		},
		pod,
	)
	res = f.IsContainerExcluded(container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerACExclude, workloadfilter.LegacyContainerACInclude}})
	assert.Equal(t, false, res)

	pod = workloadfilter.CreatePod(
		&workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "datadog-agent",
			},
		},
	)
	container = workloadfilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "nginx",
			},
		},
		pod,
	)
	res = f.IsContainerExcluded(container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerACExclude, workloadfilter.LegacyContainerACInclude}})
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
			container: workloadfilter.CreateContainer(
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
			container: workloadfilter.CreateContainer(
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
			container: workloadfilter.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
				},
				workloadfilter.CreatePod(
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
			container: workloadfilter.CreateContainer(
				&workloadmeta.Container{
					EntityMeta: workloadmeta.EntityMeta{
						Name: "nginx",
					},
				},
				workloadfilter.CreatePod(
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
			container: workloadfilter.CreateContainer(
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
			container: workloadfilter.CreateContainer(
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

			res := f.IsContainerExcluded(tt.container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerSBOM}})
			assert.Equal(t, tt.expected, res, "Container exclusion result mismatch")
		})
	}
}

func TestFilterPrecedence(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_exclude", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_include_metrics", []string{"name:dd-agent"})

	f := newFilterObject(t, mockConfig)

	container := workloadfilter.CreateContainer(
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

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("First set includes, second set not evaluated", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerMetrics}, // Includes (higher priority)
			{workloadfilter.LegacyContainerGlobal},  // Excludes (but lower priority)
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Included, res)
	})

	t.Run("First set unknown, second set exclude", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerLogs},   // Unknown, no results
			{workloadfilter.LegacyContainerGlobal}, // Excludes
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Excluded, res)
	})

	t.Run("First set unknown, second set include", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{workloadfilter.LegacyContainerLogs},    // Unknown, no results
			{workloadfilter.LegacyContainerMetrics}, // Includes
		}

		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Included, res)
	})
}

func TestEvaluateResourceNoFilters(t *testing.T) {
	mockConfig := configmock.New(t)
	f := newFilterObject(t, mockConfig)

	container := workloadfilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "no-filter",
			},
		},
		nil,
	)

	t.Run("No filter sets", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Unknown, res)
	})

	t.Run("Empty filter set", func(t *testing.T) {
		precedenceFilters := [][]workloadfilter.ContainerFilter{
			{}, {}, {}, {}, {}, {}, {}, {}, {},
		}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Unknown, res)
	})
}

func TestContainerFilterInitializationError(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("container_exclude", []string{"bad_name:nginx"})
	mockConfig.SetWithoutSource("container_include_metrics", []string{"name:dd-agent"})
	mockConfig.SetWithoutSource("ac_include", []string{"other_bad_name:nginx"})
	f := newFilterObject(t, mockConfig)

	t.Run("Properly defined filter", func(t *testing.T) {
		errs := f.GetContainerFilterInitializationErrors([]workloadfilter.ContainerFilter{workloadfilter.LegacyContainerMetrics})
		assert.Empty(t, errs, "Expected no initialization errors for properly defined filter")
	})

	t.Run("Improperly defined filter", func(t *testing.T) {
		errs := f.GetContainerFilterInitializationErrors([]workloadfilter.ContainerFilter{workloadfilter.LegacyContainerGlobal})
		assert.NotEmpty(t, errs, "Expected initialization errors for improperly defined filter")
		assert.True(t, containsErrorWithMessage(errs, "bad_name"), "Expected error message to contain the improper key 'bad_name'")
	})

	t.Run("Improperly defined filter with multiple filters", func(t *testing.T) {
		errs := f.GetContainerFilterInitializationErrors(
			append(
				workloadfilter.FlattenFilterSets(workloadfilter.GetAutodiscoveryFilters(workloadfilter.GlobalFilter)),
				workloadfilter.LegacyContainerACInclude,
			),
		)
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
	f := newFilterObject(t, mockConfig)

	container := workloadfilter.CreateContainer(
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
		// Inject a program that always errors for ContainerMetrics, but returns Included
		f.prgs[workloadfilter.ContainerType][int(workloadfilter.LegacyContainerMetrics)] = &errorInclProgram{}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Included, res)
	})

	t.Run("Exclude with error thrown", func(t *testing.T) {
		// Inject a program that always errors for ContainerMetrics, but returns Excluded
		f.prgs[workloadfilter.ContainerType][int(workloadfilter.LegacyContainerMetrics)] = &errorExclProgram{}
		res := evaluateResource(f, container, precedenceFilters)
		assert.Equal(t, workloadfilter.Excluded, res)
	})
}

func TestSpecialCharacters(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_include", []string{`name:g'oba\\r\d-0x[0-9a-fA-F]+\\n`})
	f := newFilterObject(t, mockConfig)

	container := workloadfilter.CreateContainer(
		&workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: `g'oba\r9-0xDEADBEEF\n`,
			},
		},
		nil,
	)

	res := evaluateResource(f, container, [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerGlobal}})
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
			f := newFilterObject(t, mockConfig)

			service := workloadfilter.CreateService(tt.serviceName, tt.namespace, tt.annotations)

			res := evaluateResource(f, service, tt.filters)
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
			f := newFilterObject(t, mockConfig)

			endpoint := workloadfilter.CreateEndpoint(tt.endpointName, tt.namespace, tt.annotations)

			res := evaluateResource(f, endpoint, tt.filters)
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

			f := newFilterObject(t, mockConfig)

			image := workloadfilter.CreateImage(tt.imageName)

			res := evaluateResource(f, image, [][]workloadfilter.ImageFilter{{workloadfilter.LegacyImage}})
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
