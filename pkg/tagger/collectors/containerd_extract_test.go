// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package collectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"

	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
)

var commnOCISpec = &oci.Spec{
	Process: &specs.Process{
		Env: []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOSTNAME=redis",
			"GOSU_VERSION=1.12",
			"REDIS_VERSION=6.2.5",
			"REDIS_DOWNLOAD_URL=http://download.redis.io/releases/redis-6.2.5.tar.gz",
			"REDIS_DOWNLOAD_SHA=4b9a75709a1b74b3785e20a6c158cab94cf52298aa381eea947a678a60d551ae",
			"env=dev",
			"SERVICE=my_redis_svc",
			"POD_NAME=redis",
			"KUBERNETES_PORT_443_TCP_PORT=443",
			"KUBERNETES_PORT_443_TCP_ADDR=10.96.0.1",
			"KUBERNETES_SERVICE_HOST=10.96.0.1",
			"KUBERNETES_SERVICE_PORT=443",
			"KUBERNETES_SERVICE_PORT_HTTPS=443",
			"KUBERNETES_PORT=tcp://10.96.0.1:443",
			"KUBERNETES_PORT_443_TCP=tcp://10.96.0.1:443",
			"KUBERNETES_PORT_443_TCP_PROTO=tcp",
		},
	},
}

func TestContainderdCollector_extractTags(t *testing.T) {
	mockConfig := config.Mock()
	tests := []struct {
		name      string
		loadFunc  func(*ContainderdCollector)
		resetFunc func()
		spec      *oci.Spec
		low       []string
		orch      []string
		high      []string
		std       []string
	}{
		{
			name: "no container_env_as_tags",
			loadFunc: func(c *ContainderdCollector) {
				c.envAsTags, c.globEnv = utils.InitMetadataAsTags(config.Datadog.GetStringMapString("container_env_as_tags"))
			},
			spec: commnOCISpec,
			low:  []string{},
			orch: []string{},
			high: []string{},
			std:  []string{},
		},
		{
			name: "basic container_env_as_tags",
			loadFunc: func(c *ContainderdCollector) {
				mockConfig.Set("container_env_as_tags", `{"env":"env"}`)
				c.envAsTags, c.globEnv = utils.InitMetadataAsTags(config.Datadog.GetStringMapString("container_env_as_tags"))
			},
			resetFunc: func() { mockConfig.Set("container_env_as_tags", "") },
			spec:      commnOCISpec,
			low:       []string{"env:dev"},
			orch:      []string{},
			high:      []string{},
			std:       []string{},
		},
		{
			name: "advanced container_env_as_tags",
			loadFunc: func(c *ContainderdCollector) {
				mockConfig.Set("container_env_as_tags", `{"env":"env", "SERVICE":"service", "POD_NAME":"pod_name"}`)
				c.envAsTags, c.globEnv = utils.InitMetadataAsTags(config.Datadog.GetStringMapString("container_env_as_tags"))
			},
			resetFunc: func() { mockConfig.Set("container_env_as_tags", "") },
			spec:      commnOCISpec,
			low:       []string{"env:dev", "service:my_redis_svc", "pod_name:redis"},
			orch:      []string{},
			high:      []string{},
			std:       []string{},
		},
		{
			name: "container_env_as_tags with wildcard",
			loadFunc: func(c *ContainderdCollector) {
				mockConfig.Set("container_env_as_tags", `{"REDIS*":"%%env%%"}`)
				c.envAsTags, c.globEnv = utils.InitMetadataAsTags(config.Datadog.GetStringMapString("container_env_as_tags"))
			},
			resetFunc: func() { mockConfig.Set("container_env_as_tags", "") },
			spec:      commnOCISpec,
			low: []string{
				"REDIS_VERSION:6.2.5",
				"REDIS_DOWNLOAD_URL:http://download.redis.io/releases/redis-6.2.5.tar.gz",
				"REDIS_DOWNLOAD_SHA:4b9a75709a1b74b3785e20a6c158cab94cf52298aa381eea947a678a60d551ae",
			},
			orch: []string{},
			high: []string{},
			std:  []string{},
		},
		{
			name: "nil spec",
			spec: nil,
			low:  nil,
			orch: nil,
			high: nil,
			std:  nil,
		},
		{
			name: "nil spec.process",
			spec: &specs.Spec{},
			low:  nil,
			orch: nil,
			high: nil,
			std:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ContainderdCollector{}
			if tt.loadFunc != nil {
				tt.loadFunc(c)
			}

			low, orch, high, std := c.extractTags(tt.spec)
			assert.ElementsMatch(t, tt.low, low)
			assert.ElementsMatch(t, tt.orch, orch)
			assert.ElementsMatch(t, tt.high, high)
			assert.ElementsMatch(t, tt.std, std)

			if tt.resetFunc != nil {
				tt.resetFunc()
			}
		})
	}
}
