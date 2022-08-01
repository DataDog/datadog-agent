// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

func TestParseKubeletPodlist(t *testing.T) {
	for nb, tc := range []struct {
		desc        string
		pod         *workloadmeta.KubernetesPod
		expectedCfg []integration.Config
		expectedErr ErrorMsgSet
	}{
		{
			desc: "No annotations",
			pod: &workloadmeta.KubernetesPod{
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "testName",
						ID:   "testID",
					},
				},
			},
			expectedCfg: nil,
			expectedErr: nil,
		},
		{
			desc: "v2 annotations",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.checks": `{
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
						"ad.datadoghq.com/apache.check_names":  "[\"invalid\"]",
						"ad.datadoghq.com/apache.init_configs": "[{}]",
						"ad.datadoghq.com/apache.instances":    "[{}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "kubelet:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			desc: "New + old, new takes over",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":                 "[\"http_check\"]",
						"ad.datadoghq.com/apache.init_configs":                "[{}]",
						"ad.datadoghq.com/apache.instances":                   "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"service-discovery.datadoghq.com/apache.check_names":  "[\"invalid\"]",
						"service-discovery.datadoghq.com/apache.init_configs": "[{}]",
						"service-discovery.datadoghq.com/apache.instances":    "[{}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "kubelet:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			desc: "New annotation prefix, two templates",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/apache.init_configs": "[{}]",
						"ad.datadoghq.com/apache.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/nginx.check_names":   "[\"http_check\"]",
						"ad.datadoghq.com/nginx.init_configs":  "[{}]",
						"ad.datadoghq.com/nginx.instances":     "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "kubelet:docker://3b8efe0c50e8",
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "kubelet:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			desc: "Legacy annotation prefix, two checks in one template",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"service-discovery.datadoghq.com/apache.check_names":  "[\"apache\",\"http_check\"]",
						"service-discovery.datadoghq.com/apache.init_configs": "[{},{}]",
						"service-discovery.datadoghq.com/apache.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "apache",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					Source:        "kubelet:docker://3b8efe0c50e8",
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "kubelet:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			desc: "Custom check ID",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/nginx.check.id":            "nginx-custom",
						"ad.datadoghq.com/nginx-custom.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nginx-custom.init_configs": "[{}]",
						"ad.datadoghq.com/nginx-custom.instances":    "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "kubelet:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			desc: "Non-duplicate errors",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "nginx-1752f8c774-wtjql",
					Namespace: "testNamespace",
					Annotations: map[string]string{
						"ad.datadoghq.com/nonmatching.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nonmatching.init_configs": "[{}]",
						"ad.datadoghq.com/nonmatching.instances":    "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
					{
						Name: "apache",
						ID:   "3b8efe0c50e8",
					},
				},
			},
			expectedCfg: nil,
			expectedErr: ErrorMsgSet{
				"annotation ad.datadoghq.com/nonmatching.check_names is invalid: nonmatching doesn't match a container identifier [apache nginx]":  {},
				"annotation ad.datadoghq.com/nonmatching.init_configs is invalid: nonmatching doesn't match a container identifier [apache nginx]": {},
				"annotation ad.datadoghq.com/nonmatching.instances is invalid: nonmatching doesn't match a container identifier [apache nginx]":    {},
			},
		},
		{
			desc: "One invalid config, one valid config",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{
						"ad.datadoghq.com/nginx.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/nginx.init_configs": "[{}]",
						"ad.datadoghq.com/nginx.instances":    "[{\"name\": \"nginx\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/nginx.logs":         "[{\"source\": \"nginx\" \"service\": \"nginx\"}]", // invalid json
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "nginx",
						ID:   "4ac8352d70bf1",
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"nginx\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "kubelet:docker://4ac8352d70bf1",
				},
			},
			expectedErr: ErrorMsgSet{
				"could not extract logs config: in logs: invalid character '\"' after object key:value pair": {},
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.desc), func(t *testing.T) {
			store := workloadmetatesting.NewStore()

			for _, c := range tc.pod.Containers {
				store.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   c.ID,
					},
					Runtime: workloadmeta.ContainerRuntimeDocker,
				})
			}

			m := &KubeletConfigProvider{
				workloadmetaStore: store,
				configErrors:      make(map[string]ErrorMsgSet),
				podCache: map[string]*workloadmeta.KubernetesPod{
					tc.pod.GetID().ID: tc.pod,
				},
			}

			checks, err := m.generateConfigs()
			assert.NoError(t, err)

			assert.Equal(t, len(tc.expectedCfg), len(checks))
			assert.EqualValues(t, tc.expectedCfg, checks)

			namespacedName := tc.pod.Namespace + "/" + tc.pod.Name
			assert.Equal(t, len(tc.expectedErr), len(m.configErrors[namespacedName]))
			assert.EqualValues(t, tc.expectedErr, m.configErrors[namespacedName])
		})
	}
}
