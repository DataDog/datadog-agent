// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
)

func TestProcessEvents(t *testing.T) {
	store := workloadmetatesting.NewStore()

	cp := &ContainerConfigProvider{
		workloadmetaStore: store,
		configCache:       make(map[string]map[string]integration.Config),
		configErrors:      make(map[string]ErrorMsgSet),
	}

	tests := []struct {
		name    string
		events  []workloadmeta.Event
		changes integration.ConfigChanges
	}{
		{
			name: "create config",
			events: []workloadmeta.Event{
				{
					Type:   workloadmeta.EventTypeSet,
					Entity: basicDockerContainer(),
				},
			},
			changes: integration.ConfigChanges{
				Schedule: basicDockerConfigs(),
			},
		},
		{
			name: "replace config",
			events: []workloadmeta.Event{
				{
					Type:   workloadmeta.EventTypeSet,
					Entity: basicDockerContainerSingleCheck(),
				},
			},
			changes: integration.ConfigChanges{
				Unschedule: []integration.Config{
					basicDockerConfigs()[1],
				},
			},
		},
		{
			name: "delete config",
			events: []workloadmeta.Event{
				{
					Type:   workloadmeta.EventTypeUnset,
					Entity: basicDockerContainer(),
				},
			},
			changes: integration.ConfigChanges{
				Unschedule: []integration.Config{
					basicDockerConfigs()[0],
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := cp.processEvents(workloadmeta.EventBundle{
				Events: tt.events,
			})

			assert.Equal(t, tt.changes.Schedule, changes.Schedule)
			assert.Equal(t, tt.changes.Unschedule, changes.Unschedule)
		})
	}
}

func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name                string
		entity              workloadmeta.Entity
		expectedConfigs     []integration.Config
		expectedErr         ErrorMsgSet
		containerCollectAll bool
	}{
		{
			name:            "container check",
			entity:          basicDockerContainer(),
			expectedConfigs: basicDockerConfigs(),
		},
		{
			name: "No annotations",
			entity: &workloadmeta.KubernetesPod{
				Containers: []workloadmeta.OrchestratorContainer{
					{
						Name: "testName",
						ID:   "testID",
					},
				},
			},
		},
		{
			name: "v2 annotations",
			entity: &workloadmeta.KubernetesPod{
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
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			name: "New + old, new takes over",
			entity: &workloadmeta.KubernetesPod{
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
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			name: "New annotation prefix, two templates",
			entity: &workloadmeta.KubernetesPod{
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
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Legacy annotation prefix, two checks in one template",
			entity: &workloadmeta.KubernetesPod{
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
			expectedConfigs: []integration.Config{
				{
					Name:          "apache",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://3b8efe0c50e8",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Custom check ID",
			entity: &workloadmeta.KubernetesPod{
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
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Non-duplicate errors",
			entity: &workloadmeta.KubernetesPod{
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
			expectedErr: ErrorMsgSet{
				"annotation ad.datadoghq.com/nonmatching.check_names is invalid: nonmatching doesn't match a container identifier [apache nginx]":  {},
				"annotation ad.datadoghq.com/nonmatching.init_configs is invalid: nonmatching doesn't match a container identifier [apache nginx]": {},
				"annotation ad.datadoghq.com/nonmatching.instances is invalid: nonmatching doesn't match a container identifier [apache nginx]":    {},
			},
		},
		{
			name: "One invalid config, one valid config",
			entity: &workloadmeta.KubernetesPod{
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
			expectedConfigs: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"nginx\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
					Source:        "container:docker://4ac8352d70bf1",
				},
			},
			expectedErr: ErrorMsgSet{
				"could not extract logs config: in logs: invalid character '\"' after object key:value pair": {},
			},
		},
		{
			name: "bare container, container collect all",
			entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					ID: "3b8efe0c50e8",
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedConfigs: []integration.Config{
				{
					Name:          "container_collect_all",
					Source:        "container:docker://3b8efe0c50e8",
					LogsConfig:    integration.Data("[{}]"),
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
				},
			},
			containerCollectAll: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog.Set("logs_config.container_collect_all", tt.containerCollectAll)
			defer config.Datadog.Set("logs_config.container_collect_all", false)

			store := workloadmetatesting.NewStore()

			if pod, ok := tt.entity.(*workloadmeta.KubernetesPod); ok {
				for _, c := range pod.Containers {
					store.Set(&workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   c.ID,
						},
						Runtime: workloadmeta.ContainerRuntimeDocker,
					})
				}
			}

			cp := &ContainerConfigProvider{
				workloadmetaStore: store,
				configCache:       make(map[string]map[string]integration.Config),
				configErrors:      make(map[string]ErrorMsgSet),
			}

			configs, err := cp.generateConfig(tt.entity)

			assert.Equal(t, tt.expectedConfigs, configs)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func basicDockerContainer() *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "3b8efe0c50e8",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\",\"http_check\"]",
				"com.datadoghq.ad.init_configs": "[{}, {}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}
}

func basicDockerContainerSingleCheck() *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "3b8efe0c50e8",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\"]",
				"com.datadoghq.ad.init_configs": "[{}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"}]",
			},
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}
}

func basicDockerConfigs() []integration.Config {
	return []integration.Config{
		{
			Name:          "apache",
			ADIdentifiers: []string{"docker://3b8efe0c50e8"},
			InitConfig:    integration.Data("{}"),
			Source:        "container:docker://3b8efe0c50e8",
			Instances: []integration.Data{
				integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}"),
			},
		},
		{
			Name:          "http_check",
			ADIdentifiers: []string{"docker://3b8efe0c50e8"},
			InitConfig:    integration.Data("{}"),
			Source:        "container:docker://3b8efe0c50e8",
			Instances: []integration.Data{
				integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}"),
			},
		},
	}
}
