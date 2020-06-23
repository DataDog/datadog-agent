// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package collectors

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
)

func TestParseECSClusterName(t *testing.T) {
	cases := map[string]string{
		"old-cluster-name-09": "old-cluster-name-09",
		"arn:aws:ecs:eu-central-1:601427279990:cluster/xvello-fargate": "xvello-fargate",
	}

	for value, expected := range cases {
		assert.Equal(t, expected, parseECSClusterName(value))
	}
}

func TestParseFargateRegion(t *testing.T) {
	cases := map[string]string{
		"old-cluster-name-09": "",
		"arn:aws:ecs:eu-central-1:601427279990:cluster/xvello-fargate":  "eu-central-1",
		"arn:aws:ecs:us-gov-east-1:601427279990:cluster/xvello-fargate": "us-gov-east-1",
	}

	for value, expected := range cases {
		assert.Equal(t, expected, parseFargateRegion(value))
	}
}

func TestParseMetadata(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/fargate_meta.json")
	require.NoError(t, err)
	var meta v2.Task
	err = json.Unmarshal(raw, &meta)
	require.NoError(t, err)
	require.Len(t, meta.Containers, 3)

	collector := &ECSFargateCollector{
		labelsAsTags: map[string]string{
			"highlabel": "+hightag",
			"mylabel":   "lowtag",
		},
	}
	collector.expire, err = taggerutil.NewExpire(ecsFargateExpireFreq)
	require.NoError(t, err)

	collector.expire.Update("3827da9d51f12276b4ed2d2a2dfb624b96b239b20d052b859e26c13853071e7c", time.Now())
	collector.expire.Update("unknownID", time.Now().Add(-10*time.Minute))

	expectedUpdates := []*TagInfo{
		{
			Source:      "ecs_fargate",
			Entity:      OrchestratorScopeEntityID,
			LowCardTags: []string{},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-central-1:601427279990:task/5308d232-9002-4224-97b5-e1d4843b5244",
			},
			HighCardTags: []string{},
			DeleteEntity: false,
		},
		{
			Source: "ecs_fargate",
			Entity: "container_id://1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
			LowCardTags: []string{
				"docker_image:datadog/agent-dev:xvello-process-kubelet",
				"image_name:datadog/agent-dev",
				"short_image:agent-dev",
				"image_tag:xvello-process-kubelet",
				"cluster_name:xvello-fargate",
				"ecs_cluster_name:xvello-fargate",
				"task_family:redis-datadog",
				"task_version:3",
				"ecs_container_name:datadog-agent",
				"region:eu-central-1",
			},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-central-1:601427279990:task/5308d232-9002-4224-97b5-e1d4843b5244",
			},
			HighCardTags: []string{
				"container_id:1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
				"container_name:ecs-redis-datadog-3-datadog-agent-c2a8fffa8ee8d1f6a801",
			},
			DeleteEntity: false,
		},
		{
			Source: "ecs_fargate",
			Entity: "container_id://0fc5bb7a1b29adc30997eabae1415a98fe85591eb7432c23349703a53aa43280",
			LowCardTags: []string{
				"docker_image:redis:latest",
				"image_name:redis",
				"short_image:redis",
				"image_tag:latest",
				"cluster_name:xvello-fargate",
				"ecs_cluster_name:xvello-fargate",
				"task_family:redis-datadog",
				"task_version:3",
				"ecs_container_name:redis",
				"lowtag:myvalue",
				"region:eu-central-1",
				"service:redis",
				"env:prod",
				"version:1.0",
			},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-central-1:601427279990:task/5308d232-9002-4224-97b5-e1d4843b5244",
			},
			HighCardTags: []string{
				"container_name:ecs-redis-datadog-3-redis-f6eedfd8b18a8fbe1d00",
				"hightag:value2",
				"container_id:0fc5bb7a1b29adc30997eabae1415a98fe85591eb7432c23349703a53aa43280",
			},
			DeleteEntity: false,
		},
	}

	// Diff parsing should show 2 containers
	updates, err := collector.parseMetadata(&meta, false)
	assert.NoError(t, err)
	assertTagInfoListEqual(t, expectedUpdates, updates)

	// One container expires
	expires, err := collector.expire.ComputeExpires()
	assert.NoError(t, err)
	assert.Equal(t, []string{"unknownID"}, expires)

	// Diff parsing should show 0 containers
	updates, err = collector.parseMetadata(&meta, false)
	assert.NoError(t, err)
	assert.Len(t, updates, 0)

	// Full parsing should show 3 containers
	updates, err = collector.parseMetadata(&meta, true)
	assert.NoError(t, err)
	assert.Len(t, updates, 3)
}

func TestParseMetadataV10(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/fargate_meta_v1.0.json")
	require.NoError(t, err)
	var meta v2.Task
	err = json.Unmarshal(raw, &meta)
	require.NoError(t, err)
	require.Len(t, meta.Containers, 3)

	collector := &ECSFargateCollector{}
	collector.expire, err = taggerutil.NewExpire(ecsFargateExpireFreq)
	require.NoError(t, err)

	expectedUpdates := []*TagInfo{
		{
			Source:      "ecs_fargate",
			Entity:      OrchestratorScopeEntityID,
			LowCardTags: []string{},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-west-1:172597598159:task/648ca535-cbe0-4de7-b102-28e50b81e888",
			},
			HighCardTags: []string{},
			DeleteEntity: false,
		},
		{
			Source: "ecs_fargate",
			Entity: "container_id://e8d4a9a20a0d931f8f632ec166b3f71a6ff00450aa7e99607f650e586df7d068",
			LowCardTags: []string{
				"docker_image:datadog/docker-dd-agent:latest",
				"image_name:datadog/docker-dd-agent",
				"short_image:docker-dd-agent",
				"image_tag:latest",
				"cluster_name:pierrem-test-fargate",
				"ecs_cluster_name:pierrem-test-fargate",
				"task_family:redis-datadog",
				"task_version:1",
				"ecs_container_name:dd-agent",
			},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-west-1:172597598159:task/648ca535-cbe0-4de7-b102-28e50b81e888",
			},
			HighCardTags: []string{
				"container_id:e8d4a9a20a0d931f8f632ec166b3f71a6ff00450aa7e99607f650e586df7d068",
				"container_name:ecs-redis-datadog-1-dd-agent-8085fa82d1d3ada5a601",
			},
			DeleteEntity: false,
		},
		{
			Source: "ecs_fargate",
			Entity: "container_id://c912d0f0f204360ee90ce67c0d083c3514975f149b854f38a48deac611e82e48",
			LowCardTags: []string{
				"docker_image:redis:latest",
				"image_name:redis",
				"short_image:redis",
				"image_tag:latest",
				"cluster_name:pierrem-test-fargate",
				"ecs_cluster_name:pierrem-test-fargate",
				"task_family:redis-datadog",
				"task_version:1",
				"ecs_container_name:redis",
				"service:redis",
				"env:prod",
				"version:1.0",
			},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-west-1:172597598159:task/648ca535-cbe0-4de7-b102-28e50b81e888",
			},
			HighCardTags: []string{
				"container_id:c912d0f0f204360ee90ce67c0d083c3514975f149b854f38a48deac611e82e48",
				"container_name:ecs-redis-datadog-1-redis-ce99d29f8ce998ed4a00",
			},
			DeleteEntity: false,
		},
		{
			Source: "ecs_fargate",
			Entity: "container_id://39e13ccc425e7777187a603fe33f466a18515030707c4063de1dc1b63d14d411",
			LowCardTags: []string{
				"docker_image:amazon/amazon-ecs-pause:0.1.0",
				"image_name:amazon/amazon-ecs-pause",
				"short_image:amazon-ecs-pause",
				"image_tag:0.1.0",
				"cluster_name:pierrem-test-fargate",
				"ecs_cluster_name:pierrem-test-fargate",
				"task_family:redis-datadog",
				"task_version:1",
				"ecs_container_name:~internal~ecs~pause",
			},
			OrchestratorCardTags: []string{
				"task_arn:arn:aws:ecs:eu-west-1:172597598159:task/648ca535-cbe0-4de7-b102-28e50b81e888",
			},
			HighCardTags: []string{
				"container_id:39e13ccc425e7777187a603fe33f466a18515030707c4063de1dc1b63d14d411",
				"container_name:ecs-redis-datadog-1-internalecspause-a2df9cefc2938ec19e01",
			},
			DeleteEntity: false,
		},
	}

	updates, err := collector.parseMetadata(&meta, false)
	assert.NoError(t, err)
	assertTagInfoListEqual(t, expectedUpdates, updates)
}

func TestParseExpires(t *testing.T) {
	collector := &ECSFargateCollector{}

	dead := []string{
		"1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
		"0fc5bb7a1b29adc30997eabae1415a98fe85591eb7432c23349703a53aa43280",
	}

	expected := []*TagInfo{
		{
			Source:       "ecs_fargate",
			Entity:       "container_id://1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
			DeleteEntity: true,
		},
		{
			Source:       "ecs_fargate",
			Entity:       "container_id://0fc5bb7a1b29adc30997eabae1415a98fe85591eb7432c23349703a53aa43280",
			DeleteEntity: true,
		},
	}

	out, err := collector.parseExpires(dead)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)
}
