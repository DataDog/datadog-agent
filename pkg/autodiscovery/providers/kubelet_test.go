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
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// Only testing parseKubeletPodlist, lifecycle should be tested in end-to-end test

type kubeletSuite struct {
	suite.Suite
}

func TestKubeletSuite(t *testing.T) {
	suite.Run(t, &kubeletSuite{})
}

func (s *kubeletSuite) SetupTest() {
	cache.Cache.Flush()
}

func (s *kubeletSuite) TestParseKubeletPodlist() {
	for nb, tc := range []struct {
		desc        string
		pod         *kubelet.Pod
		expectedCfg []integration.Config
	}{
		{
			desc: "No annotations",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "0",
				},
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
					UID: "1",
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":                 `["http_check"]`,
						"ad.datadoghq.com/apache.init_configs":                `[{}]`,
						"ad.datadoghq.com/apache.instances":                   `[{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
						"service-discovery.datadoghq.com/apache.check_names":  `["invalid"]`,
						"service-discovery.datadoghq.com/apache.init_configs": `[{}]`,
						"service-discovery.datadoghq.com/apache.instances":    `[{}]`,
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
					Instances:     []integration.Data{integration.Data(`{"name":"My service","timeout":1,"url":"http://%%host%%"}`)},
				},
			},
		},
		{
			desc: "New annotation prefix, two templates",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "2",
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":  `["http_check"]`,
						"ad.datadoghq.com/apache.init_configs": `[{}]`,
						"ad.datadoghq.com/apache.instances":    `[{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
						"ad.datadoghq.com/nginx.check_names":   `["http_check"]`,
						"ad.datadoghq.com/nginx.init_configs":  `[{}]`,
						"ad.datadoghq.com/nginx.instances":     `[{"name": "Other service", "url": "http://%%host_external%%", "timeout": 1}]`,
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
					Instances:     []integration.Data{integration.Data(`{"name":"My service","timeout":1,"url":"http://%%host%%"}`)},
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://4ac8352d70bf1"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"name":"Other service","timeout":1,"url":"http://%%host_external%%"}`)},
				},
			},
		},
		{
			desc: "Legacy annotation prefix, two checks in one template",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "3",
					Annotations: map[string]string{
						"service-discovery.datadoghq.com/apache.check_names":  `["apache","http_check"]`,
						"service-discovery.datadoghq.com/apache.init_configs": `[{},{}]`,
						"service-discovery.datadoghq.com/apache.instances":    `[{"apache_status_url": "http://%%host%%/server-status?auto"},{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
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
					Instances:     []integration.Data{integration.Data(`{"apache_status_url":"http://%%host%%/server-status?auto"}`)},
				},
				{
					Name:          "http_check",
					ADIdentifiers: []string{"docker://3b8efe0c50e8"},
					InitConfig:    integration.Data("{}"),
					Instances:     []integration.Data{integration.Data(`{"name":"My service","timeout":1,"url":"http://%%host%%"}`)},
				},
			},
		},
	} {
		s.T().Run(fmt.Sprintf("case %d: %s", nb, tc.desc), func(t *testing.T) {
			checks, err := parseKubeletPodlist([]*kubelet.Pod{tc.pod})
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedCfg), len(checks))
			assert.EqualValues(t, tc.expectedCfg, checks)
		})
	}
}

func (s *kubeletSuite) TestGetAutoDiscoveryPodAnnotation() {
	for _, tc := range []struct {
		pod           *kubelet.Pod
		rvalue        string
		alreadyCached bool
	}{
		{
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "12345",
				},
			},
			rvalue:        "",
			alreadyCached: false,
		},
		// Same Pod NOT cached
		{
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "12345",
				},
			},
			rvalue:        "",
			alreadyCached: false,
		},
		{
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "abc",
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":  `["http_check"]`,
						"ad.datadoghq.com/apache.init_configs": `[{}]`,
						"ad.datadoghq.com/apache.instances":    `[{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
						"ad.datadoghq.com/nginx.check_names":   `["http_check"]`,
						"ad.datadoghq.com/nginx.init_configs":  `[{}]`,
						"ad.datadoghq.com/nginx.instances":     `[{"name": "Other service", "url": "http://%%host_external%%", "timeout": 1}]`,
					},
				},
			},
			rvalue:        newPodAnnotationFormat,
			alreadyCached: false,
		},
		// Same Pod but cached
		{
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "abc",
					Annotations: map[string]string{
						"ad.datadoghq.com/apache.check_names":  `["http_check"]`,
						"ad.datadoghq.com/apache.init_configs": `[{}]`,
						"ad.datadoghq.com/apache.instances":    `[{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
						"ad.datadoghq.com/nginx.check_names":   `["http_check"]`,
						"ad.datadoghq.com/nginx.init_configs":  `[{}]`,
						"ad.datadoghq.com/nginx.instances":     `[{"name": "Other service", "url": "http://%%host_external%%", "timeout": 1}]`,
					},
				},
			},
			rvalue:        newPodAnnotationFormat,
			alreadyCached: true,
		},
		{
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "legacy",
					Annotations: map[string]string{
						"service-discovery.datadoghq.com/apache.check_names":  `["apache","http_check"]`,
						"service-discovery.datadoghq.com/apache.init_configs": `[{},{}]`,
						"service-discovery.datadoghq.com/apache.instances":    `[{"apache_status_url": "http://%%host%%/server-status?auto"},{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
					},
				},
			},
			rvalue:        legacyPodAnnotationFormat,
			alreadyCached: false,
		},
		// Same Pod but cached
		{
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					UID: "legacy",
					Annotations: map[string]string{
						"service-discovery.datadoghq.com/apache.check_names":  `["apache","http_check"]`,
						"service-discovery.datadoghq.com/apache.init_configs": `[{},{}]`,
						"service-discovery.datadoghq.com/apache.instances":    `[{"apache_status_url": "http://%%host%%/server-status?auto"},{"name": "My service", "url": "http://%%host%%", "timeout": 1}]`,
					},
				},
			},
			rvalue:        legacyPodAnnotationFormat,
			alreadyCached: true,
		},
	} {
		s.T().Run("", func(t *testing.T) {
			_, hit := cache.Cache.Get(kubeletPodAnnotationCachePrefix + tc.pod.Metadata.UID)
			assert.Equal(t, tc.alreadyCached, hit)
			assert.Equal(t, tc.rvalue, getAutoDiscoveryPodAnnotation(tc.pod))
		})
	}
}
