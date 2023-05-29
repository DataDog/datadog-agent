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
			provider := &KubeServiceConfigProvider{upToDate: true}
			provider.invalidateIfChanged(tc.old, tc.obj)

			upToDate, err := provider.IsUpToDate(ctx)
			assert.NoError(t, err)
			assert.Equal(t, !tc.invalidate, upToDate)
		})
	}
}
