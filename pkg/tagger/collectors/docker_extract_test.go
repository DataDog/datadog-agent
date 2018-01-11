// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

func TestDockerRecordsFromInspect(t *testing.T) {
	testCases := []struct {
		testName             string
		co                   *types.ContainerJSON
		toRecordEnvAsTags    map[string]string
		toRecordLabelsAsTags map[string]string
		expectedLow          []string
		expectedHigh         []string
	}{
		{
			testName: "emptyExtract",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v"},
					Labels: map[string]string{"labelKey": "labelValue"},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow:          []string{},
			expectedHigh:         []string{},
		},
		{
			testName: "extractOneLowEnv",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v"},
					Labels: map[string]string{"labelKey": "labelKey"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "becomeK"},
			toRecordLabelsAsTags: map[string]string{"labelKey": "labelValue"},
			expectedLow:          []string{"becomeK:v"},
			expectedHigh:         []string{},
		},
		{
			testName: "extractTwoLowOneHigh",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v", "l=t"},
					Labels: map[string]string{"labelKey": "labelValue"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "+becomeK", "l": "expectedLow"},
			toRecordLabelsAsTags: map[string]string{"labelkey": "labelKey"},
			expectedLow:          []string{"expectedLow:t", "labelKey:labelValue"},
			expectedHigh:         []string{"becomeK:v"},
		},
		{
			testName: "extractOneLowTwoHigh",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v", "l=t"},
					Labels: map[string]string{"labelKey": "labelValue"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "+becomeK", "l": "expectedLow"},
			toRecordLabelsAsTags: map[string]string{"labelkey": "+labelKey"},
			expectedLow:          []string{"expectedLow:t"},
			expectedHigh:         []string{"becomeK:v", "labelKey:labelValue"},
		},
		{
			testName: "extractMesosDCOS",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"MARATHON_APP_ID=/system/dd-agent",
						"CHRONOS_JOB_NAME=app1_process-orders",
						"CHRONOS_JOB_OWNER=qa",
						"MESOS_TASK_ID=system_dd-agent.dcc75b42-4b87-11e7-9a62-70b3d5800001",
					},
					Labels: map[string]string{},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"marathon_app:/system/dd-agent",
				"chronos_job:app1_process-orders",
				"chronos_job_owner:qa",
			},
			expectedHigh: []string{"mesos_task:system_dd-agent.dcc75b42-4b87-11e7-9a62-70b3d5800001"},
		},
		{
			testName: "NoValue",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"NOVALUE=",
						"AVALUE=value",
					},
					Labels: map[string]string{},
				},
			},
			toRecordEnvAsTags:    map[string]string{"avalue": "v"},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow:          []string{"v:value"},
			expectedHigh:         []string{},
		},
		{
			testName: "extractSwarmLabels",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/bin"},
					Labels: map[string]string{
						"com.docker.swarm.node.id":      "zdtab51ei97djzrpa1y2tz8li",
						"com.docker.swarm.service.id":   "tef96xrdmlj82c7nt57jdntl8",
						"com.docker.swarm.service.name": "helloworld",
						"com.docker.swarm.task":         "",
						"com.docker.swarm.task.id":      "knk1rz1szius7pvyznn9zolld",
						"com.docker.swarm.task.name":    "helloworld.1.knk1rz1szius7pvyznn9zolld",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow:          []string{"swarm_service:helloworld"},
			expectedHigh:         []string{},
		},
		{
			testName: "extractSwarmLabelsWithCustomLabelsAdds",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/bin"},
					Labels: map[string]string{
						"com.docker.swarm.node.id":      "zdtab51ei97djzrpa1y2tz8li",
						"com.docker.swarm.service.id":   "tef96xrdmlj82c7nt57jdntl8",
						"com.docker.swarm.service.name": "helloworld",
						"com.docker.swarm.task":         "",
						"com.docker.swarm.task.id":      "knk1rz1szius7pvyznn9zolld",
						"com.docker.swarm.task.name":    "helloworld.1.knk1rz1szius7pvyznn9zolld",
					},
				},
			},
			toRecordEnvAsTags: map[string]string{},
			toRecordLabelsAsTags: map[string]string{
				// Add some uncovered swarm labels to be extracted
				"com.docker.swarm.node.id":   "custom_add_swarm_node",
				"com.docker.swarm.task.name": "+custom_add_task_name",
			},
			expectedLow:  []string{"swarm_service:helloworld", "custom_add_swarm_node:zdtab51ei97djzrpa1y2tz8li"},
			expectedHigh: []string{"custom_add_task_name:helloworld.1.knk1rz1szius7pvyznn9zolld"},
		},
		{
			testName: "extractRancherLabels",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/bin"},
					Labels: map[string]string{
						"io.rancher.cni.network":             "ipsec",
						"io.rancher.cni.wait":                "true",
						"io.rancher.container.ip":            "10.42.234.7/16",
						"io.rancher.container.mac_address":   "02:f1:dd:48:4c:d9",
						"io.rancher.container.name":          "testAD-redis-1",
						"io.rancher.container.pull_image":    "always",
						"io.rancher.container.uuid":          "8e969193-2bc7-4a58-9a54-9eed44b01bb2",
						"io.rancher.environment.uuid":        "adminProject",
						"io.rancher.project.name":            "testAD",
						"io.rancher.project_service.name":    "testAD/redis",
						"io.rancher.service.deployment.unit": "06c082fc-4b66-4b6c-b098-30dbf29ed204",
						"io.rancher.service.launch.config":   "io.rancher.service.primary.launch.config",
						"io.rancher.stack.name":              "testAD",
						"io.rancher.stack_service.name":      "testAD/redis",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"rancher_service:testAD/redis",
				"rancher_stack:testAD",
			},
			expectedHigh: []string{
				"rancher_container:testAD-redis-1",
			},
		},
	}

	dc := &DockerCollector{}
	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, test.testName), func(t *testing.T) {
			dc.envAsTags = test.toRecordEnvAsTags
			dc.labelsAsTags = test.toRecordLabelsAsTags
			tags := utils.NewTagList()
			dockerExtractEnvironmentVariables(tags, test.co.Config.Env, test.toRecordEnvAsTags)
			dockerExtractLabels(tags, test.co.Config.Labels, test.toRecordLabelsAsTags)
			low, high := tags.Compute()

			// Low card tags
			assert.Equal(t, len(test.expectedLow), len(low), "test case %d", i)
			for _, lt := range test.expectedLow {
				assert.Contains(t, low, lt, "test case %d", i)
			}

			// High card tags
			assert.True(t, len(test.expectedHigh) == len(high))
			for _, ht := range test.expectedHigh {
				assert.Contains(t, high, ht, "test case %d", i)
			}
		})
	}
}
