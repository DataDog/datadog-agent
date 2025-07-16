// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	acTelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

var (
	nodename1 = "node1"
	nodename2 = "node2"
)

func TestParseKubeServiceAnnotationsForEndpoints(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := acTelemetry.NewStore(telemetry)

	for _, tc := range []struct {
		name        string
		service     *v1.Service
		expectedOut []configInfo
		hybrid      bool
	}{
		{
			name:        "nil",
			service:     nil,
			expectedOut: nil,
		},
		{
			name: "valid adv1",
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
			name: "valid adv2",
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
			name: "adv1 + ignore_autodiscovery_tags",
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
		{
			name: "adv2 + ignore_autodiscovery_tags",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/endpoints.checks": `{
							"http_check": {
								"ignore_autodiscovery_tags": true,
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
						IgnoreAutodiscoveryTags: true,
					},
					namespace: "default",
					name:      "myservice",
				},
			},
		},
		{
			name: "adv2 check + adv1 annotation + hybrid",
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
			hybrid: true,
		},
		{
			name: "adv2 check + adv1 annotation but not hybrid",
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
						IgnoreAutodiscoveryTags: false,
					},
					namespace: "default",
					name:      "myservice",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			if tc.hybrid {
				cfg.SetWithoutSource("cluster_checks.support_hybrid_ignore_ad_tags", true)
			}
			provider := kubeEndpointsConfigProvider{
				telemetryStore: telemetryStore,
			}
			cfgs := provider.parseServiceAnnotationsForEndpoints([]*v1.Service{tc.service}, cfg)
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
		t.Run(tc.name, func(t *testing.T) {
			cfgs := generateConfigs(tc.template, tc.resolveMode, tc.endpoints)
			assert.EqualValues(t, tc.expectedOut, cfgs)
		})
	}
}

func TestInvalidateOnServiceAdd(t *testing.T) {
	serviceWithoutEndpointAnnotations := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-no-endpoint-annotations",
			Namespace: "default",
			UID:       types.UID("service-no-endpoint-annotations-uid"),
			Annotations: map[string]string{
				"some.annotation": "some-value",
			},
		},
	}

	serviceWithEndpointAnnotations := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-with-endpoint-annotations",
			Namespace: "default",
			UID:       types.UID("service-with-endpoint-annotations-uid"),
			Annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"url\": \"http://%%host%%\"}]",
			},
		},
	}

	tests := []struct {
		name             string
		newService       *v1.Service
		expectedUpToDate bool
	}{
		{
			name:             "Add service without endpoint annotations",
			newService:       serviceWithoutEndpointAnnotations,
			expectedUpToDate: true,
		},
		{
			name:             "Add service with endpoint annotations",
			newService:       serviceWithEndpointAnnotations,
			expectedUpToDate: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &kubeEndpointsConfigProvider{upToDate: true}

			provider.invalidateOnServiceAdd(test.newService)

			upToDate, err := provider.IsUpToDate(context.TODO())
			require.NoError(t, err)
			assert.Equal(t, test.expectedUpToDate, upToDate)
		})
	}

}

func TestInvalidateOnServiceUpdate(t *testing.T) {
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
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			provider := &kubeEndpointsConfigProvider{upToDate: true}
			provider.invalidateOnServiceUpdate(tc.old, tc.obj)

			upToDate, err := provider.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, !tc.invalidate, upToDate)
		})
	}
}

func TestInvalidateOnServiceDelete(t *testing.T) {
	serviceWithoutAnnotations := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-no-annotations",
			Namespace: "default",
			UID:       types.UID("service-no-annotations-uid"),
		},
	}

	serviceWithAnnotations := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-with-annotations",
			Namespace: "default",
			UID:       types.UID("service-with-annotations-uid"),
			Annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names":  "[\"http_check\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"url\": \"http://%%host%%\"}]",
			},
		},
	}

	tests := []struct {
		name               string
		monitoredEndpoints map[string]bool
		deletedService     *v1.Service
		expectedUpToDate   bool
	}{
		{
			name: "Delete service that had monitored endpoints",
			monitoredEndpoints: map[string]bool{
				apiserver.EntityForEndpoints(serviceWithAnnotations.Namespace, serviceWithAnnotations.Name, ""): true,
			},
			deletedService:   serviceWithAnnotations,
			expectedUpToDate: false,
		},
		{
			name:               "Delete service that did not have monitored endpoints",
			monitoredEndpoints: map[string]bool{},
			deletedService:     serviceWithoutAnnotations,
			expectedUpToDate:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &kubeEndpointsConfigProvider{
				upToDate:           true,
				monitoredEndpoints: test.monitoredEndpoints,
			}

			provider.invalidateOnServiceDelete(test.deletedService)

			upToDate, err := provider.IsUpToDate(context.TODO())
			require.NoError(t, err)
			assert.Equal(t, test.expectedUpToDate, upToDate)
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
			provider.invalidateOnEndpointsUpdate(tc.first, tc.second)

			upToDate, err := provider.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tc.upToDate, upToDate)
		})
	}
}

func TestGetConfigErrors_KubeEndpoints(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := acTelemetry.NewStore(telemetry)

	serviceWithErrors := v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: kubernetes.ServiceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withErrors",
			Namespace: "default",
			UID:       "123",
			Annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names":  "[\"some_check\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"url\" \"%%host%%\"}]", // Invalid JSON (missing ":" after "url")
			},
		},
	}

	endpointsOfServiceWithErrors := v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withErrors",
			Namespace: "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "10.0.0.1",
					},
				},
			},
		},
	}

	serviceWithoutErrors := v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: kubernetes.ServiceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withoutErrors",
			Namespace: "default",
			UID:       "456",
			Annotations: map[string]string{ // No errors
				"ad.datadoghq.com/endpoints.check_names":  "[\"some_check\"]",
				"ad.datadoghq.com/endpoints.init_configs": "[{}]",
				"ad.datadoghq.com/endpoints.instances":    "[{\"url\": \"%%host%%\"}]",
			},
		},
	}

	endpointsOfServiceWithoutErrors := v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withoutErrors",
			Namespace: "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP: "10.0.0.2",
					},
				},
			},
		},
	}

	tests := []struct {
		name                          string
		currentErrors                 map[string]providerTypes.ErrorMsgSet
		collectedServicesAndEndpoints []runtime.Object
		expectedNumCollectedConfigs   int
		expectedErrorsAfterCollect    map[string]providerTypes.ErrorMsgSet
	}{
		{
			name:          "case without errors",
			currentErrors: map[string]providerTypes.ErrorMsgSet{},
			collectedServicesAndEndpoints: []runtime.Object{
				&serviceWithoutErrors,
				&endpointsOfServiceWithoutErrors,
			},
			expectedNumCollectedConfigs: 1,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
		{
			name: "endpoint that has been deleted and had errors",
			currentErrors: map[string]providerTypes.ErrorMsgSet{
				"kube_endpoint_uid://default/deletedService/": {"error1": struct{}{}},
			},
			collectedServicesAndEndpoints: []runtime.Object{
				&serviceWithoutErrors,
				&endpointsOfServiceWithoutErrors,
			},
			expectedNumCollectedConfigs: 1,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
		{
			name: "endpoint with error that has been fixed",
			currentErrors: map[string]providerTypes.ErrorMsgSet{
				"kube_endpoint_uid://default/withoutErrors/": {"error1": struct{}{}},
			},
			collectedServicesAndEndpoints: []runtime.Object{
				&serviceWithoutErrors,
				&endpointsOfServiceWithoutErrors,
			},
			expectedNumCollectedConfigs: 1,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
		{
			name:          "endpoint that did not have an error but now does",
			currentErrors: map[string]providerTypes.ErrorMsgSet{},
			collectedServicesAndEndpoints: []runtime.Object{
				&serviceWithErrors,
				&endpointsOfServiceWithErrors,
			},
			expectedNumCollectedConfigs: 0,
			expectedErrorsAfterCollect: map[string]providerTypes.ErrorMsgSet{
				"kube_endpoint_uid://default/withErrors/": {
					"could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key": struct{}{},
				},
			},
		},
		{
			name: "endpoint that had an error and still does",
			currentErrors: map[string]providerTypes.ErrorMsgSet{
				"kube_endpoint_uid://default/withErrors/": {
					"could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key": struct{}{},
				},
			},
			collectedServicesAndEndpoints: []runtime.Object{
				&serviceWithErrors,
				&endpointsOfServiceWithErrors,
			},
			expectedNumCollectedConfigs: 0,
			expectedErrorsAfterCollect: map[string]providerTypes.ErrorMsgSet{
				"kube_endpoint_uid://default/withErrors/": {
					"could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key": struct{}{},
				},
			},
		},
		{
			name:                          "nothing collected",
			currentErrors:                 map[string]providerTypes.ErrorMsgSet{},
			collectedServicesAndEndpoints: []runtime.Object{},
			expectedNumCollectedConfigs:   0,
			expectedErrorsAfterCollect:    map[string]providerTypes.ErrorMsgSet{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset(test.collectedServicesAndEndpoints...)
			factory := informers.NewSharedInformerFactory(kubeClient, time.Duration(0))
			serviceLister := factory.Core().V1().Services().Lister()
			endpointsLister := factory.Core().V1().Endpoints().Lister()

			stop := make(chan struct{})
			defer close(stop)
			factory.Start(stop)
			factory.WaitForCacheSync(stop)

			provider := kubeEndpointsConfigProvider{
				serviceLister:      serviceLister,
				endpointsLister:    endpointsLister,
				configErrors:       test.currentErrors,
				monitoredEndpoints: make(map[string]bool),
				telemetryStore:     telemetryStore,
			}

			configs, err := provider.Collect(context.TODO())
			require.NoError(t, err)
			require.Len(t, configs, test.expectedNumCollectedConfigs)
			assert.Equal(t, test.expectedErrorsAfterCollect, provider.GetConfigErrors())
		})
	}
}
