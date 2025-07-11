// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestProcessService(t *testing.T) {
	ksvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             types.UID("test"),
			Labels: map[string]string{
				"tags.datadoghq.com/env":     "dev",
				"tags.datadoghq.com/version": "1.0.0",
				"tags.datadoghq.com/service": "my-http-service",
			},
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
			Name:      "myservice",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{Name: "test1", Port: 123},
				{Name: "test2", Port: 126},
			},
		},
	}

	svc := processService(ksvc, workloadfilterfxmock.SetupMockFilter(t))
	assert.Equal(t, "kube_service://default/myservice", svc.GetServiceID())

	adID := svc.GetADIdentifiers()
	assert.Equal(t, []string{"kube_service://default/myservice"}, adID)

	hosts, err := svc.GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"cluster": "10.0.0.1"}, hosts)

	ports, err := svc.GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "test1"}, {126, "test2"}}, ports)

	tags, err := svc.GetTags()
	assert.NoError(t, err)
	expectedTags := []string{
		"kube_service:myservice",
		"kube_namespace:default",
		"env:dev",
		"service:my-http-service",
		"version:1.0.0",
	}
	sort.Strings(expectedTags)
	sort.Strings(tags)
	assert.Equal(t, expectedTags, tags)
}

func TestServicesDiffer(t *testing.T) {
	for name, tc := range map[string]struct {
		first  *v1.Service
		second *v1.Service
		result bool
	}{
		"Same resversion": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
			},
			result: false,
		},
		"Change resversion, same spec": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			result: false,
		},
		"Change IP": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.10",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Change port number": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 124},
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Remove port": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Add annotation": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Remove annotation": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: true,
		},
		"Add standard tags": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
				},
			},
			result: true,
		},
		"Remove standard tags": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Labels: map[string]string{
						"tags.datadoghq.com/env": "dev",
					},
				},
			},
			result: true,
		},
		"Same standard tags": {
			first: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			second: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test2", Port: 126},
					},
				},
			},
			result: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.result, servicesDiffer(tc.first, tc.second))
		})
	}
}

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name      string
		targetAll bool
		ksvc      *v1.Service
		want      bool
	}{
		{
			name:      "no targetAll, with dd annotations",
			targetAll: false,
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
					Name:      "myservice",
					Namespace: "default",
				},
			},
			want: false,
		},
		{
			name:      "no targetAll, with prom annotations",
			targetAll: false,
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
					},
					Name:      "myservice",
					Namespace: "default",
				},
			},
			want: false,
		},
		{
			name:      "with targetAll, no annotations",
			targetAll: true,
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					Name:        "myservice",
					Namespace:   "default",
				},
			},
			want: false,
		},
		{
			name:      "no targetAll, no annotations",
			targetAll: false,
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
					Name:        "myservice",
					Namespace:   "default",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &KubeServiceListener{
				promInclAnnot:     getPrometheusIncludeAnnotations(),
				targetAllServices: tt.targetAll,
			}
			assert.Equal(t, tt.want, l.shouldIgnore(tt.ksvc))
		})
	}
}

func TestHasFilterKubeServices(t *testing.T) {
	tests := []struct {
		name            string
		ksvc            *v1.Service
		metricsExcluded bool
		globalExcluded  bool
		want            bool
		filterScope     workloadfilter.Scope
	}{
		{
			name: "metrics excluded is true",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: true,
			globalExcluded:  false,
			want:            true,
			filterScope:     workloadfilter.MetricsFilter,
		},
		{
			name: "metrics excluded is false",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: false,
			globalExcluded:  true,
			want:            false,
			filterScope:     workloadfilter.MetricsFilter,
		},
		{
			name: "metrics excluded is true with logs filter",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: true,
			globalExcluded:  true,
			want:            false,
			filterScope:     workloadfilter.LogsFilter,
		},
		{
			name: "metrics excluded is false with logs filter",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: false,
			globalExcluded:  false,
			want:            false,
			filterScope:     workloadfilter.LogsFilter,
		},
		{
			name: "global excluded is true",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: false,
			globalExcluded:  true,
			want:            true,
			filterScope:     workloadfilter.GlobalFilter,
		},
		{
			name: "global excluded is false",
			ksvc: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("test"),
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "dev",
						"tags.datadoghq.com/version": "1.0.0",
						"tags.datadoghq.com/service": "my-http-service",
					},
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "test1", Port: 123},
						{Name: "test2", Port: 126},
					},
				},
			},
			metricsExcluded: true,
			globalExcluded:  false,
			want:            false,
			filterScope:     workloadfilter.GlobalFilter,
		},
	}
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := processService(tt.ksvc, filterStore)
			svc.metricsExcluded = tt.metricsExcluded
			svc.globalExcluded = tt.globalExcluded
			isFilter := svc.HasFilter(tt.filterScope)
			assert.Equal(t, isFilter, tt.want)
		})
	}
}

func TestKubeServiceFiltering(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_exclude_metrics", []string{"kube_namespace:excluded-namespace"})
	mockConfig.SetWithoutSource("container_exclude", []string{"name:global-excluded"})
	mockFilterStore := workloadfilterfxmock.SetupMockFilter(t)

	// Create test services with different scenarios
	testCases := []struct {
		name                string
		service             *v1.Service
		expectedMetricsExcl bool
		expectedGlobalExcl  bool
	}{
		{
			name: "normal service - not excluded",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "normal-service",
					Namespace: "default",
					UID:       types.UID("normal-uid"),
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []v1.ServicePort{
						{Name: "http", Port: 80},
					},
				},
			},
			expectedMetricsExcl: false,
			expectedGlobalExcl:  false,
		},
		{
			name: "service in excluded namespace - metrics excluded",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-in-excluded-ns",
					Namespace: "excluded-namespace",
					UID:       types.UID("excluded-ns-uid"),
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.2",
					Ports: []v1.ServicePort{
						{Name: "http", Port: 80},
					},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  false,
		},
		{
			name: "globally excluded service",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-excluded",
					Namespace: "default",
					UID:       types.UID("global-excluded-uid"),
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.3",
					Ports: []v1.ServicePort{
						{Name: "http", Port: 80},
					},
				},
			},
			expectedMetricsExcl: false,
			expectedGlobalExcl:  true,
		},
		{
			name: "service with AD annotations - metrics excluded",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ad-excluded",
					Namespace: "default",
					UID:       types.UID("ad-excluded-uid"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names": "[\"http_check\"]",
						"ad.datadoghq.com/metrics_exclude":     "true",
						"ad.datadoghq.com/exclude":             "false",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.4",
					Ports: []v1.ServicePort{
						{Name: "http", Port: 80},
					},
				},
			},
			expectedMetricsExcl: true,
			expectedGlobalExcl:  false,
		},
		{
			name: "service with global AD exclusion",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ad-global-excluded",
					Namespace: "default",
					UID:       types.UID("ad-global-excluded-uid"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names": "[\"http_check\"]",
						"ad.datadoghq.com/exclude":             "true",
					},
				},
				Spec: v1.ServiceSpec{
					ClusterIP: "10.0.0.5",
					Ports: []v1.ServicePort{
						{Name: "http", Port: 80},
					},
				},
			},
			expectedMetricsExcl: false,
			expectedGlobalExcl:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call processService directly with the mock filter store
			svc := processService(tc.service, mockFilterStore)

			// Verify that service was created
			assert.NotNil(t, svc, "Service should be created")

			// Verify filtering results
			assert.Equal(t, tc.expectedMetricsExcl, svc.metricsExcluded,
				"Expected metricsExcluded to be %v for service %s/%s",
				tc.expectedMetricsExcl, tc.service.Namespace, tc.service.Name)
			assert.Equal(t, tc.expectedGlobalExcl, svc.globalExcluded,
				"Expected globalExcluded to be %v for service %s/%s",
				tc.expectedGlobalExcl, tc.service.Namespace, tc.service.Name)
		})
	}
}
