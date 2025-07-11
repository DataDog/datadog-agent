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
	"go.uber.org/atomic"
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
)

func TestParseKubeServiceAnnotations(t *testing.T) {
	telemetry := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := acTelemetry.NewStore(telemetry)

	for _, tc := range []struct {
		name        string
		service     *v1.Service
		expectedOut []integration.Config
		hybrid      bool
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
					Name:      "svc",
					Namespace: "ns",
				},
			},
			expectedOut: []integration.Config{
				{
					Name:                    "http_check",
					ADIdentifiers:           []string{"kube_service://ns/svc"},
					InitConfig:              integration.Data("{}"),
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:            true,
					Source:                  "kube_services:kube_service://ns/svc",
					IgnoreAutodiscoveryTags: false,
				},
			},
		},
		{
			name: "valid service annotations v2 only",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My service",
										"url": "http://%%host%%",
										"timeout": 1
									}
								]
							}
						}`,
					},
					Name:      "svc",
					Namespace: "ns",
				},
			},
			expectedOut: []integration.Config{
				{
					Name:                    "http_check",
					ADIdentifiers:           []string{"kube_service://ns/svc"},
					InitConfig:              integration.Data("{}"),
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:            true,
					Source:                  "kube_services:kube_service://ns/svc",
					IgnoreAutodiscoveryTags: false,
				},
			},
		},
		{
			name: "valid service annotations v2 only + ignore adv2 tags",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My service",
										"url": "http://%%host%%",
										"timeout": 1
									}
								],
								"ignore_autodiscovery_tags": true
							}
						}`,
					},
					Name:      "svc",
					Namespace: "ns",
				},
			},
			expectedOut: []integration.Config{
				{
					Name:                    "http_check",
					ADIdentifiers:           []string{"kube_service://ns/svc"},
					InitConfig:              integration.Data("{}"),
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:            true,
					Source:                  "kube_services:kube_service://ns/svc",
					IgnoreAutodiscoveryTags: true,
				},
			},
		},
		{
			name: "ignore AD tags",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.check_names":               "[\"http_check\"]",
						"ad.datadoghq.com/service.init_configs":              "[{}]",
						"ad.datadoghq.com/service.instances":                 "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/service.ignore_autodiscovery_tags": "true",
					},
					Name:      "svc",
					Namespace: "ns",
				},
			},
			expectedOut: []integration.Config{
				{
					Name:                    "http_check",
					ADIdentifiers:           []string{"kube_service://ns/svc"},
					InitConfig:              integration.Data("{}"),
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:            true,
					Source:                  "kube_services:kube_service://ns/svc",
					IgnoreAutodiscoveryTags: true,
				},
			},
		},
		{
			name: "adv2 check with adv1 ignore_autodiscovery_tags in hybrid mode",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My service",
										"url": "http://%%host%%",
										"timeout": 1
									}
								]
							}
						}`,
						"ad.datadoghq.com/service.ignore_autodiscovery_tags": "true",
					},
					Name:      "svc",
					Namespace: "ns",
				},
			},
			expectedOut: []integration.Config{
				{
					Name:                    "http_check",
					ADIdentifiers:           []string{"kube_service://ns/svc"},
					InitConfig:              integration.Data("{}"),
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:            true,
					Source:                  "kube_services:kube_service://ns/svc",
					IgnoreAutodiscoveryTags: true,
				},
			},
			hybrid: true,
		},
		{
			name: "adv2 check with adv1 ignore_autodiscovery_tags in non hybrid mode",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("test"),
					Annotations: map[string]string{
						"ad.datadoghq.com/service.checks": `{
							"http_check": {
								"instances": [
									{
										"name": "My service",
										"url": "http://%%host%%",
										"timeout": 1
									}
								]
							}
						}`,
						"ad.datadoghq.com/service.ignore_autodiscovery_tags": "true",
					},
					Name:      "svc",
					Namespace: "ns",
				},
			},
			expectedOut: []integration.Config{
				{
					Name:                    "http_check",
					ADIdentifiers:           []string{"kube_service://ns/svc"},
					InitConfig:              integration.Data("{}"),
					Instances:               []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					ClusterCheck:            true,
					Source:                  "kube_services:kube_service://ns/svc",
					IgnoreAutodiscoveryTags: false,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			if tc.hybrid {
				cfg.SetWithoutSource("cluster_checks.support_hybrid_ignore_ad_tags", true)
			}

			provider := KubeServiceConfigProvider{
				telemetryStore: telemetryStore,
				upToDate:       atomic.NewBool(false),
			}
			cfgs, _ := provider.parseServiceAnnotations([]*v1.Service{tc.service}, cfg)
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
			provider := &KubeServiceConfigProvider{upToDate: atomic.NewBool(true)}
			provider.invalidateIfChanged(tc.old, tc.obj)

			upToDate, err := provider.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, !tc.invalidate, upToDate)
		})
	}
}

func TestGetConfigErrors_KubeServices(t *testing.T) {
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
				"ad.datadoghq.com/service.check_names":  "[\"some_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"url\" \"%%host%%\"}]", // Invalid JSON (missing ":" after "url")
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
				"ad.datadoghq.com/service.check_names":  "[\"some_check\"]",
				"ad.datadoghq.com/service.init_configs": "[{}]",
				"ad.datadoghq.com/service.instances":    "[{\"url\": \"%%host%%\"}]",
			},
		},
	}

	tests := []struct {
		name                        string
		currentErrors               map[string]providerTypes.ErrorMsgSet
		collectedServices           []runtime.Object
		expectedNumCollectedConfigs int
		expectedErrorsAfterCollect  map[string]providerTypes.ErrorMsgSet
	}{
		{
			name:          "case without errors",
			currentErrors: map[string]providerTypes.ErrorMsgSet{},
			collectedServices: []runtime.Object{
				&serviceWithoutErrors,
			},
			expectedNumCollectedConfigs: 1,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
		{
			name: "service that has been deleted and had errors",
			currentErrors: map[string]providerTypes.ErrorMsgSet{
				"kube_service://default/deletedService": {"error1": struct{}{}},
			},
			collectedServices: []runtime.Object{
				&serviceWithoutErrors,
			},
			expectedNumCollectedConfigs: 1,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
		{
			name: "service with error that has been fixed",
			currentErrors: map[string]providerTypes.ErrorMsgSet{
				"kube_service://default/withoutErrors": {"error1": struct{}{}},
			},
			collectedServices: []runtime.Object{
				&serviceWithoutErrors,
			},
			expectedNumCollectedConfigs: 1,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
		{
			name:          "service that did not have an error but now does",
			currentErrors: map[string]providerTypes.ErrorMsgSet{},
			collectedServices: []runtime.Object{
				&serviceWithErrors,
			},
			expectedNumCollectedConfigs: 0,
			expectedErrorsAfterCollect: map[string]providerTypes.ErrorMsgSet{
				"kube_service://default/withErrors": {
					"could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key": struct{}{},
				},
			},
		},
		{
			name: "service that had an error and still does",
			currentErrors: map[string]providerTypes.ErrorMsgSet{
				"kube_service://default/withErrors": {
					"could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key": struct{}{},
				},
			},
			collectedServices: []runtime.Object{
				&serviceWithErrors,
			},
			expectedNumCollectedConfigs: 0,
			expectedErrorsAfterCollect: map[string]providerTypes.ErrorMsgSet{
				"kube_service://default/withErrors": {
					"could not extract checks config: in instances: failed to unmarshal JSON: invalid character '\"' after object key": struct{}{},
				},
			},
		},
		{
			name:                        "nothing collected",
			currentErrors:               map[string]providerTypes.ErrorMsgSet{},
			collectedServices:           []runtime.Object{},
			expectedNumCollectedConfigs: 0,
			expectedErrorsAfterCollect:  map[string]providerTypes.ErrorMsgSet{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset(test.collectedServices...)
			factory := informers.NewSharedInformerFactory(kubeClient, time.Duration(0))
			lister := factory.Core().V1().Services().Lister()

			stop := make(chan struct{})
			defer close(stop)
			factory.Start(stop)
			factory.WaitForCacheSync(stop)

			provider := KubeServiceConfigProvider{
				lister:         lister,
				configErrors:   test.currentErrors,
				telemetryStore: telemetryStore,
				upToDate:       atomic.NewBool(false),
			}

			configs, err := provider.Collect(context.TODO())
			require.NoError(t, err)
			require.Len(t, configs, test.expectedNumCollectedConfigs)
			assert.Equal(t, test.expectedErrorsAfterCollect, provider.GetConfigErrors())
		})
	}
}
