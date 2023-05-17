// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestProcessService(t *testing.T) {
	ctx := context.Background()
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

	svc := processService(ksvc)
	assert.Equal(t, "kube_service://default/myservice", svc.GetServiceID())

	adID, err := svc.GetADIdentifiers(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service://default/myservice"}, adID)

	hosts, err := svc.GetHosts(ctx)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"cluster": "10.0.0.1"}, hosts)

	ports, err := svc.GetPorts(ctx)
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
