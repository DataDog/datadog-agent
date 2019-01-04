// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
		expectedOrch         []string
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
			expectedOrch:         []string{},
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
			expectedOrch:         []string{},
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
			expectedOrch:         []string{},
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
			expectedOrch:         []string{},
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
			expectedOrch: []string{
				"mesos_task:system_dd-agent.dcc75b42-4b87-11e7-9a62-70b3d5800001",
			},
			expectedHigh: []string{},
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
			expectedOrch:         []string{},
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
			expectedOrch:         []string{},
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
			expectedLow:  []string{"custom_add_swarm_node:zdtab51ei97djzrpa1y2tz8li", "swarm_service:helloworld"},
			expectedOrch: []string{},
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
			expectedOrch: []string{},
			expectedHigh: []string{
				"rancher_container:testAD-redis-1",
			},
		},
		{
			testName: "extractNomad",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"NOMAD_TASK_NAME=test-task",
						"NOMAD_JOB_NAME=test-job",
						"NOMAD_GROUP_NAME=test-group",
					},
					Labels: map[string]string{},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"nomad_task:test-task",
				"nomad_job:test-job",
				"nomad_group:test-group",
			},
			expectedOrch: []string{},
			expectedHigh: []string{},
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
			low, orchestrator, high := tags.Compute()

			// Low card tags
			assert.Equal(t, len(test.expectedLow), len(low), "test case %d", i)
			for _, lt := range test.expectedLow {
				assert.Contains(t, low, lt, "test case %d", i)
			}

			// orchestrator card tags
			assert.Equal(t, len(test.expectedOrch), len(orchestrator), "test case %d", i)
			for _, ot := range test.expectedOrch {
				assert.Contains(t, orchestrator, ot, "test case %d", i)
			}

			// High card tags
			assert.True(t, len(test.expectedHigh) == len(high))
			for _, ht := range test.expectedHigh {
				assert.Contains(t, high, ht, "test case %d", i)
			}
		})
	}
}

func TestDockerExtractImage(t *testing.T) {
	for nb, tc := range []struct {
		testName     string
		co           types.ContainerJSON
		resolveMap   map[string]string
		expectedTags []string
	}{
		{
			testName: "Swarm image tag",
			co: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					Image: "sha256:3199480a61205f9ebe93672d0125a05f88eec656c8ab78de893edeafe2db4eb1",
				},
				Config: &container.Config{
					Image: "dockercloud/haproxy:1.6.7@sha256:8c4ed4049f55de49cbc8d03d057a5a7e8d609c264bb75b59a04470db1d1c5121",
				},
			},
			resolveMap: map[string]string{
				"sha256:3199480a61205f9ebe93672d0125a05f88eec656c8ab78de893edeafe2db4eb1": "fail",
			},
			expectedTags: []string{
				"docker_image:dockercloud/haproxy:1.6.7",
				"image_name:dockercloud/haproxy",
				"short_image:haproxy",
				"image_tag:1.6.7",
			},
		},
		{
			testName: "Docker compose - locally built image",
			co: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					Image: "sha256:6c3435d803d4bc2926b8803d6fa48fbeafffc163c65ca7d31f523d7510fac53d",
				},
				Config: &container.Config{
					Image: "jmx_jmx-template_in_labels---fail", // We should not take this one (no sha), keep existing behaviour
				},
			},
			resolveMap: map[string]string{
				"sha256:6c3435d803d4bc2926b8803d6fa48fbeafffc163c65ca7d31f523d7510fac53d": "jmx_jmx-template_in_labels:latest",
			},
			expectedTags: []string{
				"docker_image:jmx_jmx-template_in_labels:latest",
				"image_name:jmx_jmx-template_in_labels",
				"short_image:jmx_jmx-template_in_labels",
				"image_tag:latest",
			},
		},
		{
			testName: "Kubernetes and Nomad",
			co: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					Image: "sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358",
				},
				Config: &container.Config{
					Image: "sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358",
				},
			},
			resolveMap: map[string]string{
				"sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358": "gcr.io/google_containers/kube-proxy:v1.10.6-gke.2",
			},
			expectedTags: []string{
				"docker_image:gcr.io/google_containers/kube-proxy:v1.10.6-gke.2",
				"image_name:gcr.io/google_containers/kube-proxy",
				"short_image:kube-proxy",
				"image_tag:v1.10.6-gke.2",
			},
		},
		{
			testName: "Mesos",
			co: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					Image: "sha256:73265529441f483ed7a5d485be5ffdee5f8496323e445549e31dfae3d3e1bfa7",
				},
				Config: &container.Config{
					Image: "datadog/agent:latest---fail", // We should not take this one (no sha), keep existing behaviour
				},
			},
			resolveMap: map[string]string{
				"sha256:73265529441f483ed7a5d485be5ffdee5f8496323e445549e31dfae3d3e1bfa7": "datadog/agent:latest",
			},
			expectedTags: []string{
				"docker_image:datadog/agent:latest",
				"image_name:datadog/agent",
				"short_image:agent",
				"image_tag:latest",
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.testName), func(t *testing.T) {
			resolve := func(image string) (string, error) { return tc.resolveMap[image], nil }
			tags := utils.NewTagList()
			dockerExtractImage(tags, tc.co, resolve)
			low, _, _ := tags.Compute()

			assert.Equal(t, len(tc.expectedTags), len(low))
			for _, lt := range tc.expectedTags {
				assert.Contains(t, low, lt)
			}
		})
	}
}
