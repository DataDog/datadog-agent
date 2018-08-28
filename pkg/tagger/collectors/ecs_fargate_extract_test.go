// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package collectors

import (
	"io/ioutil"
	"testing"
	"time"

	json "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
)

func TestParseECSClusterName(t *testing.T) {
	cases := map[string]string{
		"old-cluster-name-09":                                          "old-cluster-name-09",
		"arn:aws:ecs:eu-central-1:601427279990:cluster/xvello-fargate": "xvello-fargate",
	}

	for value, expected := range cases {
		assert.Equal(t, expected, parseECSClusterName(value))
	}
}

func TestParseMetadata(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/fargate_meta.json")
	require.NoError(t, err)
	var meta ecs.TaskMetadata
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
			Source: "ecs_fargate",
			Entity: "docker://1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
			LowCardTags: []string{
				"docker_image:datadog/agent-dev:xvello-process-kubelet",
				"image_name:datadog/agent-dev",
				"short_image:agent-dev",
				"image_tag:xvello-process-kubelet",
				"cluster_name:xvello-fargate",
				"task_family:redis-datadog",
				"task_version:3",
				"ecs_container_name:datadog-agent",
			},
			HighCardTags: []string{
				"container_id:1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
				"container_name:ecs-redis-datadog-3-datadog-agent-c2a8fffa8ee8d1f6a801",
			},
			DeleteEntity: false,
		},
		{
			Source: "ecs_fargate",
			Entity: "docker://0fc5bb7a1b29adc30997eabae1415a98fe85591eb7432c23349703a53aa43280",
			LowCardTags: []string{
				"docker_image:redis:latest",
				"image_name:redis",
				"short_image:redis",
				"image_tag:latest",
				"cluster_name:xvello-fargate",
				"task_family:redis-datadog",
				"task_version:3",
				"ecs_container_name:redis",
				"lowtag:myvalue",
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
	updates, err := collector.parseMetadata(meta, false)
	assert.NoError(t, err)
	assertTagInfoListEqual(t, expectedUpdates, updates)

	// One container expires
	expires, err := collector.expire.ComputeExpires()
	assert.NoError(t, err)
	assert.Equal(t, []string{"unknownID"}, expires)

	// Diff parsing should show 0 containers
	updates, err = collector.parseMetadata(meta, false)
	assert.NoError(t, err)
	assert.Len(t, updates, 0)

	// Full parsing should show 3 containers
	updates, err = collector.parseMetadata(meta, true)
	assert.NoError(t, err)
	assert.Len(t, updates, 3)
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
			Entity:       "docker://1cd08ea0fc13ee643fa058a8e184861661eb29325c7df59ccc543597018ffcd4",
			DeleteEntity: true,
		},
		{
			Source:       "ecs_fargate",
			Entity:       "docker://0fc5bb7a1b29adc30997eabae1415a98fe85591eb7432c23349703a53aa43280",
			DeleteEntity: true,
		},
	}

	out, err := collector.parseExpires(dead)
	assert.NoError(t, err)
	assert.Equal(t, expected, out)
}
