// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
		expectedStandard     []string
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
			expectedStandard:     []string{},
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
			expectedStandard:     []string{},
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
			expectedStandard:     []string{},
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
			expectedStandard:     []string{},
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
			expectedHigh:     []string{},
			expectedStandard: []string{},
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
			expectedStandard:     []string{},
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
			expectedStandard:     []string{},
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
			expectedLow:      []string{"custom_add_swarm_node:zdtab51ei97djzrpa1y2tz8li", "swarm_service:helloworld"},
			expectedOrch:     []string{},
			expectedHigh:     []string{"custom_add_task_name:helloworld.1.knk1rz1szius7pvyznn9zolld"},
			expectedStandard: []string{},
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
			expectedStandard: []string{},
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
			expectedOrch:     []string{},
			expectedHigh:     []string{},
			expectedStandard: []string{},
		},
		{
			testName: "Standard tags in labels",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Labels: map[string]string{
						"com.datadoghq.tags.service": "redis",
						"com.datadoghq.tags.env":     "dev",
						"com.datadoghq.tags.version": "0.0.1",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
			},
			expectedOrch: []string{},
			expectedHigh: []string{},
			expectedStandard: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
			},
		},
		{
			testName: "Standard tags in env variables",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"DD_SERVICE=redis",
						"DD_ENV=dev",
						"DD_VERSION=0.0.1",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
			},
			expectedOrch: []string{},
			expectedHigh: []string{},
			expectedStandard: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
			},
		},
		{
			testName: "Same standard tags from labels and env variables => no duplicates",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"DD_SERVICE=redis",
						"DD_ENV=dev",
						"DD_VERSION=0.0.1",
					},
					Labels: map[string]string{
						"com.datadoghq.tags.service": "redis",
						"com.datadoghq.tags.env":     "dev",
						"com.datadoghq.tags.version": "0.0.1",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
			},
			expectedOrch: []string{},
			expectedHigh: []string{},
			expectedStandard: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
			},
		},
		{
			testName: "Different standard tags from labels and env variables => no override",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"DD_SERVICE=redis",
						"DD_ENV=dev",
						"DD_VERSION=0.0.1",
					},
					Labels: map[string]string{
						"com.datadoghq.tags.service": "redis-db",
						"com.datadoghq.tags.env":     "staging",
						"com.datadoghq.tags.version": "0.0.2",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
				"service:redis-db",
				"env:staging",
				"version:0.0.2",
			},
			expectedOrch: []string{},
			expectedHigh: []string{},
			expectedStandard: []string{
				"service:redis",
				"env:dev",
				"version:0.0.1",
				"service:redis-db",
				"env:staging",
				"version:0.0.2",
			},
		},
		{
			testName: "extractCustomLabels",
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{"PATH=/bin"},
					Labels: map[string]string{
						"com.datadoghq.ad.tags": "[\"adTestKey:adTestVal1\", \"adTestKey:adTestVal2\"]",
					},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow:          []string{},
			expectedOrch:         []string{},
			expectedHigh:         []string{"adTestKey:adTestVal1", "adTestKey:adTestVal2"},
			expectedStandard:     []string{},
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
			low, orchestrator, high, standard := tags.Compute()

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

			// Standard  tags
			assert.True(t, len(test.expectedStandard) == len(standard))
			for _, st := range test.expectedStandard {
				assert.Contains(t, standard, st, "test case %d", i)
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
		{
			testName: "Some Nomad Setup",
			co: types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					Image: "sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358",
				},
				Config: &container.Config{
					Image: "quay.io/foo/bar:3451-be4c56f",
				},
			},
			resolveMap: map[string]string{
				"sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358": "sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358",
			},
			expectedTags: []string{
				"docker_image:sha256:380b233f1574da39494e2b36e65f262214fe158af5ae7a94d026b7a4e46fa358",
				"image_name:quay.io/foo/bar",
				"short_image:bar",
				"image_tag:3451-be4c56f",
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.testName), func(t *testing.T) {
			resolve := func(co types.ContainerJSON) (string, error) { return tc.resolveMap[co.Image], nil }
			tags := utils.NewTagList()
			dockerExtractImage(tags, tc.co, resolve)
			low, _, _, _ := tags.Compute()

			assert.Equal(t, len(tc.expectedTags), len(low))
			for _, lt := range tc.expectedTags {
				assert.Contains(t, low, lt)
			}
		})
	}
}
