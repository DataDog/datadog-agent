// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestParseKubeServiceAnnotations(t *testing.T) {
	for _, tc := range []struct {
		name        string
		service     *v1.Service
		expectedOut []integration.Config
	}{
		{
			name:        "nil input",
			service:     nil,
			expectedOut: nil,
		},
		{
			name: "valid service annotations only",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs": "[{}]",
						"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
			},
			expectedOut: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"kube_service://test"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
				},
			},
		},
		{
			name: "valid endpoints annotations only",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:       types.UID("test"),
					Namespace: "default",
					Name:      "myservice",
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
			},
			expectedOut: []integration.Config{
				{
					Name:          "etcd",
					ADIdentifiers: []string{"kube_endpoint://default/myservice"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:2379/metrics\",\"use_preview\":\"true\"}")},
					ClusterCheck:  false,
				},
			},
		},
		{
			name: "valid service and endpoints annotations",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:       types.UID("test"),
					Namespace: "default",
					Name:      "myservice",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
			},
			expectedOut: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"kube_service://test"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
					EndpointsChecks: []integration.Config{
						{
							Name:          "etcd",
							ADIdentifiers: []string{"kube_endpoint://default/myservice"},
							InitConfig:    integration.Data("{}"),
							Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:2379/metrics\",\"use_preview\":\"true\"}")},
							ClusterCheck:  false,
						},
					},
				},
				{
					Name:          "etcd",
					ADIdentifiers: []string{"kube_endpoint://default/myservice"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:2379/metrics\",\"use_preview\":\"true\"}")},
					ClusterCheck:  false,
				},
			},
		},
		{
			name: "invalid service and endpoints annotations",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\" \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\" \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
			},
			expectedOut: nil,
		},
		{
			name: "valid service annotations, invalid endpoints annotations",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:       types.UID("test"),
					Namespace: "default",
					Name:      "myservice",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\" \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
			},
			expectedOut: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"kube_service://test"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
				},
			},
		},
		{
			name: "invalid service annotations, valid endpoints annotations",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:       types.UID("test"),
					Namespace: "default",
					Name:      "myservice",
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":   "[{}]",
						"ad.datadoghq.com/service.instances":      "[{\"name\" \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
					},
				},
			},
			expectedOut: []integration.Config{
				{
					Name:          "etcd",
					ADIdentifiers: []string{"kube_endpoint://default/myservice"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"prometheus_url\":\"http://%%host%%:2379/metrics\",\"use_preview\":\"true\"}")},
					ClusterCheck:  false,
				},
			},
		},
	} {
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
			cfgs, _ := parseServiceAnnotations([]*v1.Service{tc.service})
			assert.EqualValues(t, tc.expectedOut, cfgs)
		})
	}
}

func TestInvalidateIfChanged(t *testing.T) {
	s88 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "88",
		},
	}
	s89 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "89",
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
	}
	s90 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "90",
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
	}
	s91 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "91",
		},
	}
	s92 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "92",
			Namespace:       "default",
			Name:            "myendpoint",
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs":   "[{}]",
				"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"true\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
			},
		},
	}
	s93 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "93",
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":    "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs":   "[{}]",
				"ad.datadoghq.com/service.instances":      "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
				"ad.datadoghq.com/endpoints.check_names":  "[\"etcd\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"use_preview\": \"false\", \"prometheus_url\": \"http://%%host%%:2379/metrics\"}]",
			},
		},
	}
	s94 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "94",
			Annotations: map[string]string{
				"ad.datadoghq.com/service.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
	}
	invalid := &v1.Pod{}

	for _, tc := range []struct {
		old        interface{}
		obj        interface{}
		invalidate bool
	}{
		{
			// Invalid input
			old:        nil,
			obj:        nil,
			invalidate: false,
		},
		{
			// Sync on missed create
			old:        nil,
			obj:        s88,
			invalidate: true,
		},
		{
			// Edit, annotations added
			old:        s88,
			obj:        s89,
			invalidate: true,
		},
		{
			// Informer resync, don't invalidate
			old:        s89,
			obj:        s89,
			invalidate: false,
		},
		{
			// Invalid input, don't invalidate
			old:        s89,
			obj:        invalid,
			invalidate: false,
		},
		{
			// Edit but same annotations
			old:        s89,
			obj:        s90,
			invalidate: false,
		},
		{
			// Edit, annotations removed
			old:        s89,
			obj:        s91,
			invalidate: true,
		},
		{
			// Edit, add endpoints annotations
			old:        s89,
			obj:        s92,
			invalidate: true,
		},
		{
			// Edit endpoints annotations
			old:        s92,
			obj:        s93,
			invalidate: true,
		},
		{
			// Edit endpoints annotations removed
			old:        s92,
			obj:        s94,
			invalidate: true,
		},
	} {
		t.Run(fmt.Sprintf(""), func(t *testing.T) {
			provider := &KubeServiceConfigProvider{upToDate: true}
			provider.invalidateIfChanged(tc.old, tc.obj)

			upToDate, err := provider.IsUpToDate()
			assert.NoError(t, err)
			assert.Equal(t, !tc.invalidate, upToDate)
		})
	}
}
