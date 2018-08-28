// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// Only testing parseKubeletPodlist, lifecycle should be tested in end-to-end test

func TestParseKubeletPodlist(t *testing.T) {
	for nb, tc := range []struct {
		desc        string
		pod         *kubelet.Pod
		expectedCfg []integration.Config
	}{
		{
			desc: "No annotations",
			pod: &kubelet.Pod{
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "testName",
							ID:   "testID",
						},
					},
				},
			},
			expectedCfg: nil,
		},
		{
			desc: "New + old, new takes over",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":                 "[\"http_check\"]",
						"ad.datadoghq.com/apache.init_configs":                "[{}]",
						"ad.datadoghq.com/apache.instances":                   "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"service-discovery.datadoghq.com/apache.check_names":  "[\"invalid\"]",
						"service-discovery.datadoghq.com/apache.init_configs": "[{}]",
						"service-discovery.datadoghq.com/apache.instances":    "[{}]",
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "apache",
							ID:   "docker://3b8efe0c50e8",
						},
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
				},
			},
		},
		{
			desc: "New annotation prefix, two templates",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":  "[\"http_check\"]",
						"ad.datadoghq.com/apache.init_configs": "[{}]",
						"ad.datadoghq.com/apache.instances":    "[{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
						"ad.datadoghq.com/nginx.check_names":   "[\"http_check\"]",
						"ad.datadoghq.com/nginx.init_configs":  "[{}]",
						"ad.datadoghq.com/nginx.instances":     "[{\"name\": \"Other service\", \"url\": \"http://%%host_external%%\", \"timeout\": 1}]",
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "apache",
							ID:   "docker://3b8efe0c50e8",
						},
						{
							Name: "nginx",
							ID:   "docker://4ac8352d70bf1",
						},
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"Other service\",\"timeout\":1,\"url\":\"http://%%host_external%%\"}")},
				},
			},
		},
		{
			desc: "Legacy annotation prefix, two checks in one template",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Annotations: map[string]string{
						"service-discovery.datadoghq.com/apache.check_names":  "[\"apache\",\"http_check\"]",
						"service-discovery.datadoghq.com/apache.init_configs": "[{},{}]",
						"service-discovery.datadoghq.com/apache.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"},{\"name\": \"My service\", \"url\": \"http://%%host%%\", \"timeout\": 1}]",
					},
				},
				Status: kubelet.Status{
					Containers: []kubelet.ContainerStatus{
						{
							Name: "apache",
							ID:   "docker://3b8efe0c50e8",
						},
					},
				},
			},
			expectedCfg: []integration.Config{
				{
					Name:          "apache",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"apache_status_url\":\"http://%%host%%/server-status?auto\"}")},
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data("{\"name\":\"My service\",\"timeout\":1,\"url\":\"http://%%host%%\"}")},
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.desc), func(t *testing.T) {
			checks, err := parseKubeletPodlist([]*kubelet.Pod{tc.pod})
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedCfg), len(checks))
			for _, config := range checks {
				expectedCfg := getConfigByName(tc.expectedCfg, config.Name, config.ADIdentifiers)
				require.NotNil(t, expectedCfg)

				assert.Equal(t, expectedCfg.Name, config.Name)
				assert.EqualValues(t, expectedCfg.ADIdentifiers, config.ADIdentifiers)
				assert.JSONEq(t, string(expectedCfg.InitConfig), string(config.InitConfig))

				// NOTE: this could break if Instances has a different order
				for jdx, instance := range config.Instances {
					assert.JSONEq(t, string(expectedCfg.Instances[jdx]), string(instance))
				}
			}
		})
	}
}
