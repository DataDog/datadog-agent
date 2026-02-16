// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	provTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseKubeServiceAnnotationsForEndpointSlices(t *testing.T) {
	configmock.New(t)

	for _, tc := range []struct {
		name        string
		service     *v1.Service
		expectedOut []configInfoSlices
	}{
		{
			name:        "nil service",
			service:     nil,
			expectedOut: nil,
		},
		{
			name: "valid annotations",
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
			expectedOut: []configInfoSlices{
				{
					tpl: integration.Config{
						Name:                    "http_check",
						ADIdentifiers:           []string{"default/myservice"},
						InitConfig:              integration.Data("{}"),
						Instances:               []integration.Data{integration.Data("{\"name\":\"My endpoint\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
						ClusterCheck:            false,
						Source:                  "kube_endpoints:" + apiserver.EntityForEndpoints("default", "myservice", ""),
						IgnoreAutodiscoveryTags: false,
					},
					namespace:   "default",
					serviceName: "myservice",
				},
			},
		},
		{
			name: "service without endpoint annotations",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"some.other.annotation": "value",
					},
					Name:      "myservice",
					Namespace: "default",
				},
			},
			expectedOut: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &kubeEndpointSlicesConfigProvider{
				configErrors: make(map[string]provTypes.ErrorMsgSet),
			}

			configs := provider.parseServiceAnnotationsForEndpointSlices([]*v1.Service{tc.service})

			if tc.expectedOut == nil {
				assert.Equal(t, 0, len(configs))
			} else {
				require.Equal(t, len(tc.expectedOut), len(configs))
				for i, expected := range tc.expectedOut {
					assert.Equal(t, expected.namespace, configs[i].namespace)
					assert.Equal(t, expected.serviceName, configs[i].serviceName)
					assert.Equal(t, expected.tpl.Name, configs[i].tpl.Name)
					assert.Equal(t, expected.tpl.ADIdentifiers, configs[i].tpl.ADIdentifiers)
					assert.Equal(t, expected.tpl.Source, configs[i].tpl.Source)
				}
			}
		})
	}
}

func TestGenerateConfigFromSlice(t *testing.T) {
	port123 := int32(123)
	port126 := int32(126)
	portName123 := "port123"
	portName126 := "port126"
	nodeName1 := "node1"
	nodeName2 := "node2"

	for _, tc := range []struct {
		name        string
		resolveMode endpointResolveMode
		slice       *discv1.EndpointSlice
		template    integration.Config
		expectedOut []integration.Config
	}{
		{
			name:        "nil EndpointSlice",
			slice:       nil,
			template:    integration.Config{},
			expectedOut: []integration.Config{{}},
		},
		{
			name:        "EndpointSlice without TargetRef",
			resolveMode: "auto",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myservice-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName123, Port: &port123},
					{Name: &portName126, Port: &port126},
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
			name:        "EndpointSlice with TargetRef and auto mode",
			resolveMode: "auto",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myservice-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  types.UID("pod-uid-1"),
						},
						NodeName: &nodeName1,
					},
					{
						Addresses: []string{"10.0.0.2"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  types.UID("pod-uid-2"),
						},
						NodeName: &nodeName2,
					},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName123, Port: &port123},
					{Name: &portName126, Port: &port126},
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
			name:        "EndpointSlice with TargetRef but resolve=ip",
			resolveMode: "ip",
			slice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myservice-abc",
					Namespace: "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "myservice",
					},
				},
				Endpoints: []discv1.Endpoint{
					{
						Addresses: []string{"10.0.0.1"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  types.UID("pod-uid-1"),
						},
						NodeName: &nodeName1,
					},
					{
						Addresses: []string{"10.0.0.2"},
						TargetRef: &v1.ObjectReference{
							Kind: "Pod",
							UID:  types.UID("pod-uid-2"),
						},
						NodeName: &nodeName2,
					},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName123, Port: &port123},
					{Name: &portName126, Port: &port126},
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
			cfgs := generateConfigFromSlice(tc.template, tc.resolveMode, tc.slice, "default", "myservice")
			assert.EqualValues(t, tc.expectedOut, cfgs)
		})
	}
}

func TestEndpointSlice_InvalidateOnServiceAdd(t *testing.T) {
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
			provider := &kubeEndpointSlicesConfigProvider{upToDate: true}

			provider.invalidateOnServiceAdd(test.newService)

			upToDate, err := provider.IsUpToDate(t.Context())
			require.NoError(t, err)
			assert.Equal(t, test.expectedUpToDate, upToDate)
		})
	}
}

func TestEndpointSlice_InvalidateOnServiceDelete(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-1",
			Namespace: "default",
			UID:       types.UID("service-1-uid"),
		},
	}

	tests := []struct {
		name              string
		monitoredServices map[string]bool
		deletedService    *v1.Service
		expectedUpToDate  bool
	}{
		{
			name: "Delete service that had monitored endpoints",
			monitoredServices: map[string]bool{
				"default/service-1": true,
			},
			deletedService:   service,
			expectedUpToDate: false,
		},
		{
			name:              "Delete service that did not have monitored endpoints",
			monitoredServices: map[string]bool{},
			deletedService:    service,
			expectedUpToDate:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &kubeEndpointSlicesConfigProvider{
				upToDate:          true,
				monitoredServices: test.monitoredServices,
			}

			provider.invalidateOnServiceDelete(test.deletedService)

			upToDate, err := provider.IsUpToDate(t.Context())
			require.NoError(t, err)
			assert.Equal(t, test.expectedUpToDate, upToDate)
		})
	}
}

func TestEndpointSlice_InvalidateOnServiceUpdate(t *testing.T) {
	s88 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "88",
			Name:            "test-svc",
			Namespace:       "default",
		},
	}
	s89 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "89",
			Name:            "test-svc",
			Namespace:       "default",
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
			Name:            "test-svc",
			Namespace:       "default",
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
			Name:            "test-svc",
			Namespace:       "default",
		},
	}
	invalid := &v1.Pod{}

	for _, tc := range []struct {
		name       string
		old        interface{}
		obj        interface{}
		invalidate bool
	}{
		{
			name:       "Invalid input, nil old and obj",
			old:        nil,
			obj:        nil,
			invalidate: false,
		},
		{
			name:       "Sync on missed create",
			old:        nil,
			obj:        s88,
			invalidate: true,
		},
		{
			name:       "Edit, annotations added",
			old:        s88,
			obj:        s89,
			invalidate: true,
		},
		{
			name:       "Informer resync (same resource version)",
			old:        s89,
			obj:        s89,
			invalidate: false,
		},
		{
			name:       "Invalid input (wrong type)",
			old:        s89,
			obj:        invalid,
			invalidate: false,
		},
		{
			name:       "Edit, same annotations",
			old:        s89,
			obj:        s90,
			invalidate: false,
		},
		{
			name:       "Edit, annotations removed",
			old:        s89,
			obj:        s91,
			invalidate: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &kubeEndpointSlicesConfigProvider{upToDate: true}
			provider.invalidateOnServiceUpdate(tc.old, tc.obj)

			upToDate, err := provider.IsUpToDate(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, !tc.invalidate, upToDate)
		})
	}
}

func TestEndpointSlice_InvalidateOnEndpointSliceUpdate(t *testing.T) {
	port80 := int32(80)
	portName := "http"

	testService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
			Annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names": `["http_check"]`,
			},
		},
	}

	tests := []struct {
		name             string
		oldSlice         *discv1.EndpointSlice
		newSlice         *discv1.EndpointSlice
		expectedUpToDate bool
	}{
		{
			name: "Same resource version - no invalidation",
			oldSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
			},
			newSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
			},
			expectedUpToDate: true,
		},
		{
			name: "Different resource version, same endpoints - no invalidation",
			oldSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			newSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedUpToDate: true,
		},
		{
			name: "Endpoint added - invalidate",
			oldSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			newSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedUpToDate: false,
		},
		{
			name: "IP address changed - invalidate",
			oldSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "123",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.2"}},
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			newSlice: &discv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "124",
					Namespace:       "default",
					Labels: map[string]string{
						"kubernetes.io/service-name": "test-svc",
					},
				},
				Endpoints: []discv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}},
					{Addresses: []string{"10.0.0.3"}}, // Changed IP
				},
				Ports: []discv1.EndpointPort{
					{Name: &portName, Port: &port80},
				},
			},
			expectedUpToDate: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create provider with fake client
			objs := []runtime.Object{testService}
			client := fake.NewClientset(objs...)
			informerFactory := informers.NewSharedInformerFactory(client, 0)

			provider := &kubeEndpointSlicesConfigProvider{
				serviceLister:       informerFactory.Core().V1().Services().Lister(),
				endpointSliceLister: informerFactory.Discovery().V1().EndpointSlices().Lister(),
				upToDate:            true,
				monitoredServices:   map[string]bool{"default/test-svc": true},
			}

			stopCh := make(chan struct{})
			informerFactory.Start(stopCh)
			informerFactory.WaitForCacheSync(stopCh)
			close(stopCh)

			// Trigger update
			provider.invalidateOnEndpointSliceUpdate(tc.oldSlice, tc.newSlice)

			// Check result
			upToDate, err := provider.IsUpToDate(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.expectedUpToDate, upToDate)
		})
	}
}

func TestHasEndpointSliceAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name: "has .checks annotation",
			annotations: map[string]string{
				"ad.datadoghq.com/endpoints.checks": `{}`,
			},
			expected: true,
		},
		{
			name: "has .instances annotation",
			annotations: map[string]string{
				"ad.datadoghq.com/endpoints.instances": `[]`,
			},
			expected: true,
		},
		{
			name: "has .check_names annotation",
			annotations: map[string]string{
				"ad.datadoghq.com/endpoints.check_names": `[]`,
			},
			expected: true,
		},
		{
			name: "has legacy annotation",
			annotations: map[string]string{
				"service-discovery.datadoghq.com/endpoints.checks": `{}`,
			},
			expected: true,
		},
		{
			name:        "no annotations",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name: "unrelated service check annotation",
			annotations: map[string]string{
				"ad.datadoghq.com/service.checks": `{}`,
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tc.annotations,
				},
			}
			result := hasEndpointSliceAnnotations(svc)
			assert.Equal(t, tc.expected, result)
		})
	}
}
