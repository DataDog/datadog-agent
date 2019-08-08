// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestProcessService(t *testing.T) {
	ksvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "123",
			UID:             types.UID("test"),
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

	svc := processService(ksvc, true)
	assert.Equal(t, "kube_service_uid://test", svc.GetEntity())
	assert.Equal(t, integration.Before, svc.GetCreationTime())

	adID, err := svc.GetADIdentifiers()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service_uid://test"}, adID)

	hosts, err := svc.GetHosts()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"cluster": "10.0.0.1"}, hosts)

	ports, err := svc.GetPorts()
	assert.NoError(t, err)
	assert.Equal(t, []ContainerPort{{123, "test1"}, {126, "test2"}}, ports)

	tags, err := svc.GetTags()
	assert.NoError(t, err)
	assert.Equal(t, []string{"kube_service:myservice", "kube_namespace:default"}, tags)

	svc = processService(ksvc, false)
	assert.Equal(t, integration.After, svc.GetCreationTime())
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
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.result, servicesDiffer(tc.first, tc.second))
		})
	}
}
