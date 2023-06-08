// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

var (
	nodename1 = "node1"
	nodename2 = "node2"
)

func TestParseKubeServiceAnnotationsForEndpoints(t *testing.T) {
	for _, tc := range []struct {
		service     *v1.Service
		expectedOut []configInfo
	}{
		{
			service:     nil,
			expectedOut: nil,
		},
		{
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/endpoints.init_configs": "[{}]",
						"ad.datadoghq.com/endpoints.instances":    "[{\"name\": \"My endpoint\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
					Name:      "myservice",
					Namespace: "default",
				},
			},
			expectedOut: []configInfo{
				{
					tpl: integration.Config{
						Name:                    "http_check",
						ADIdentifiers:           []string{"kube_endpoint_uid://default/myservice/"},
						InitConfig:              integration.Data("{}"),
						Instances:               []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
						ClusterCheck:            false,
						Source:                  "kube_endpoints:kube_endpoint_uid://default/myservice/",
						IgnoreAutodiscoveryTags: false,
					},
					namespace: "default",
					name:      "myservice",
				},
			},
		},
		{
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My endpoint",
										"url": "http://%%host%%",
										"timeout": 1
									}
								]
							}
						}`,
					},
					Name:      "myservice",
					Namespace: "default",
				},
			},
			expectedOut: []configInfo{
				{
					tpl: integration.Config{
						Name:                    "http_check",
						ADIdentifiers:           []string{"kube_endpoint_uid://default/myservice/"},
						InitConfig:              integration.Data("{}"),
						Instances:               []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
						ClusterCheck:            false,
						Source:                  "kube_endpoints:kube_endpoint_uid://default/myservice/",
						IgnoreAutodiscoveryTags: false,
					},
					namespace: "default",
					name:      "myservice",
				},
			},
		},
		{
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.check_names":               "[\"http_check\"]",
						"ad.datadoghq.com/endpoints.init_configs":              "[{}]",
						"ad.datadoghq.com/endpoints.instances":                 "[{\"name\": \"My endpoint\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/endpoints.ignore_autodiscovery_tags": "true",
					},
					Name:      "myservice",
					Namespace: "default",
				},
			},
			expectedOut: []configInfo{
				{
					tpl: integration.Config{
						Name:                    "http_check",
						ADIdentifiers:           []string{"kube_endpoint_uid://default/myservice/"},
						InitConfig:              integration.Data("{}"),
						Instances:               []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
						ClusterCheck:            false,
						Source:                  "kube_endpoints:kube_endpoint_uid://default/myservice/",
						IgnoreAutodiscoveryTags: true,
					},
					namespace: "default",
					name:      "myservice",
				},
			},
		},
	} {
		t.Run(fmt.Sprintf(""), func(t *testing.T) {
			cfgs := parseServiceAnnotationsForEndpoints([]*v1.Service{tc.service})
			assert.EqualValues(t, tc.expectedOut, cfgs)
		})
	}
}

func TestGenerateConfigs(t *testing.T) {
	for _, tc := range []struct {
		name        string
		resolveMode endpointResolveMode
		endpoints   *v1.Endpoints
		template    integration.Config
		expectedOut []integration.Config
	}{
		{
			name:        "nil kubernetes Endpoints",
			endpoints:   nil,
			template:    integration.Config{},
			expectedOut: []integration.Config{{}},
		},
		{
			name:        "Endpoints without podRef",
			resolveMode: "auto",
			endpoints: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("endpoints-uid"),
					Name:            "myservice",
					Namespace:       "default",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1"},
							{IP: "10.0.0.2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port123", Port: 123},
							{Name: "port126", Port: 126},
						},
					},
				},
			},
			template: integration.Config{
				Name:          "http_check",
				ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/"},
				InitConfig:    integration.Data("{}"),
				Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
				ClusterCheck:  false,
			},
			expectedOut: []integration.Config{
				{
					ServiceID:     "kube_endpoint_uid://default/myservice/10.0.0.1",
					Name:          "http_check",
					ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/10.0.0.1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
				},
				{
					ServiceID:     "kube_endpoint_uid://default/myservice/10.0.0.2",
					Name:          "http_check",
					ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/10.0.0.2"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
				},
			},
		},
		{
			name:        "Endpoints with podRef",
			resolveMode: "unknown",
			endpoints: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("endpoints-uid"),
					Name:            "myservice",
					Namespace:       "default",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "testhost1", NodeName: &nodename1, TargetRef: &v1.ObjectReference{
								UID:  types.UID("pod-uid-1"),
								Kind: "Pod",
							}},
							{IP: "10.0.0.2", Hostname: "testhost2", NodeName: &nodename2, TargetRef: &v1.ObjectReference{
								UID:  types.UID("pod-uid-2"),
								Kind: "Pod",
							}},
						},
						Ports: []v1.EndpointPort{
							{Name: "port123", Port: 123},
							{Name: "port126", Port: 126},
						},
					},
				},
			},
			template: integration.Config{
				Name:          "http_check",
				ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/"},
				InitConfig:    integration.Data("{}"),
				Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
				ClusterCheck:  false,
			},
			expectedOut: []integration.Config{
				{
					ServiceID:     "kube_endpoint_uid://default/myservice/10.0.0.1",
					Name:          "http_check",
					ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/10.0.0.1", "kubernetes_pod://pod-uid-1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
					NodeName:      "node1",
				},
				{
					ServiceID:     "kube_endpoint_uid://default/myservice/10.0.0.2",
					Name:          "http_check",
					ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/10.0.0.2", "kubernetes_pod://pod-uid-2"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
					NodeName:      "node2",
				},
			},
		},
		{
			name:        "Endpoints with podRef but with resolve=ip",
			resolveMode: "ip",
			endpoints: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					UID:             types.UID("endpoints-uid"),
					Name:            "myservice",
					Namespace:       "default",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "testhost1", NodeName: &nodename1, TargetRef: &v1.ObjectReference{
								UID:  types.UID("pod-uid-1"),
								Kind: "Pod",
							}},
							{IP: "10.0.0.2", Hostname: "testhost2", NodeName: &nodename2, TargetRef: &v1.ObjectReference{
								UID:  types.UID("pod-uid-2"),
								Kind: "Pod",
							}},
						},
						Ports: []v1.EndpointPort{
							{Name: "port123", Port: 123},
							{Name: "port126", Port: 126},
						},
					},
				},
			},
			template: integration.Config{
				Name:          "http_check",
				ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/"},
				InitConfig:    integration.Data("{}"),
				Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
				ClusterCheck:  false,
			},
			expectedOut: []integration.Config{
				{
					ServiceID:     "kube_endpoint_uid://default/myservice/10.0.0.1",
					Name:          "http_check",
					ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/10.0.0.1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
					NodeName:      "",
				},
				{
					ServiceID:     "kube_endpoint_uid://default/myservice/10.0.0.2",
					Name:          "http_check",
					ADIdentifiers: []string{"kube_endpoint_uid://default/myservice/10.0.0.2"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:  true,
					NodeName:      "",
				},
			},
		},
	} {
		t.Run(fmt.Sprintf(tc.name), func(t *testing.T) {
			cfgs := generateConfigs(tc.template, tc.resolveMode, tc.endpoints)
			assert.EqualValues(t, tc.expectedOut, cfgs)
		})
	}
}

func TestInvalidateIfChangedService(t *testing.T) {
	s88 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "88",
		},
	}
	s89 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "89",
			Annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"name\": \"My endpoint\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
	}
	s90 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "90",
			Annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"name\": \"My endpoint\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
	}
	s91 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "91",
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
	} {
		t.Run(fmt.Sprintf(""), func(t *testing.T) {
			ctx := context.Background()
			provider := &kubeEndpointsConfigProvider{upToDate: true}
			provider.invalidateIfChangedService(tc.old, tc.obj)

			upToDate, err := provider.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, !tc.invalidate, upToDate)
		})
	}
}

func TestInvalidateIfChangedEndpoints(t *testing.T) {
	for name, tc := range map[string]struct {
		first    *v1.Endpoints
		second   *v1.Endpoints
		upToDate bool
	}{
		"Same resversion": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
			},
			upToDate: true,
		},
		"Change resversion, same subsets": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			upToDate: true,
		},
		"Change IP": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.3", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			upToDate: false,
		},
		"Change IP for not monitored Endpoints": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "notmonitored",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "notmonitored",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.3", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			upToDate: true,
		},
		"Change Hostname": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.3", Hostname: "host3"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			upToDate: false,
		},
		"Change port number": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 126},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			upToDate: false,
		},
		"Remove IP": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			upToDate: false,
		},
		"Remove port": {
			first: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
							{Name: "port2", Port: 124},
						},
					},
				},
			},
			second: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Name:            "myservice",
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: "10.0.0.1", Hostname: "host1"},
							{IP: "10.0.0.2", Hostname: "host2"},
						},
						Ports: []v1.EndpointPort{
							{Name: "port1", Port: 123},
						},
					},
				},
			},
			upToDate: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			provider := &kubeEndpointsConfigProvider{
				upToDate: true,
				monitoredEndpoints: map[string]bool{
					apiserver.EntityForEndpoints("default", "myservice", ""): true,
				},
			}
			provider.invalidateIfChangedEndpoints(tc.first, tc.second)

			upToDate, err := provider.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tc.upToDate, upToDate)
		})
	}
}
