// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hosttags provides access to host tags
package hosttags

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/oracle"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

func setupTest(t *testing.T) (model.Config, context.Context) {
	retrySleepTime = 0
	t.Cleanup(func() {
		retrySleepTime = 1 * time.Second
		getProvidersDefinitionsFunc = getProvidersDefinitions
	})

	mockConfig := configmock.New(t)
	mockConfig.SetInTest("autoconfig_from_environment", false)
	// avoid querying the cloud providers metadata endpoints
	setupCloudProvider(t, "")
	return mockConfig, context.Background()
}

// setupCloudProvider mocks cloudproviders.DetectCloudProvider so that it returns the given cloud provider name,
// an empty name meaning that no cloud provider was detected.
func setupCloudProvider(t *testing.T, cloudProviderName string) {
	cloudproviders.Mock(t, cloudProviderName, "", "", "")
}

func TestGet(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3"})
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetEmptyHostTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	// Get should never return a nil value under System even when there are no host tags
	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{}, hostTags.System)
}

func TestGetPrivateActionRunnerTag(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		wantTag bool
	}{
		{name: "enabled", enabled: true, wantTag: true},
		{name: "disabled", enabled: false, wantTag: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig, ctx := setupTest(t)
			mockConfig.SetInTest("private_action_runner.enabled", tt.enabled)

			hostTags := Get(ctx, false, mockConfig)
			assert.Equal(t, tt.wantTag, slices.Contains(hostTags.System, "private_action_runner_enabled:true"))
		})
	}
}

func TestGetWithSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("tag_value_split_separator", map[string]string{"kafka_partition": ","})
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0", "kafka_partition:1", "kafka_partition:2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetWithoutSplits(t *testing.T) {
	mockConfig, ctx := setupTest(t)

	mockConfig.SetInTest("tag_value_split_separator", map[string]string{"kafka_partition": ";"})
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3", "kafka_partition:0,1,2"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"kafka_partition:0,1,2", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestGetWithEnv(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag3", "env:prod"})
	mockConfig.SetInTest("env", "preprod")

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"env:preprod", "env:prod", "tag1:value1", "tag2", "tag3"}, hostTags.System)
}

func TestMarshalEmptyHostTags(t *testing.T) {
	tags := &Tags{
		System:              []string{},
		GoogleCloudPlatform: []string{},
	}

	marshaled, _ := json.Marshal(tags)
	// `System` should be marshaled as an empty list
	assert.Equal(t, string(marshaled), `{"system":[]}`)
}

func TestCombineExtraTags(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("tags", []string{"tag1:value1", "tag2", "tag4"})
	mockConfig.SetInTest("extra_tags", []string{"tag1:value2", "tag3", "tag4"})

	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"tag1:value1", "tag1:value2", "tag2", "tag3", "tag4"}, hostTags.System)
}

func TestGetWithoutEUDM(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("infrastructure_mode", "full")

	hostTags := Get(ctx, false, mockConfig)
	for _, tag := range hostTags.System {
		assert.NotContains(t, tag, "infra_mode:")
		assert.NotContains(t, tag, "os_name:")
		assert.NotContains(t, tag, "os_version:")
		assert.NotContains(t, tag, "cpu_model:")
		assert.NotContains(t, tag, "device_model:")
		assert.NotContains(t, tag, "hostid:")
		assert.NotContains(t, tag, "total_memory_gb:")
	}
}

func TestGetInfraModeTags(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"none", "infra_mode:none"},
		{"basic", "infra_mode:basic"},
		{"cloud_cost_only", "infra_mode:cloud_cost_only"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			mockConfig, ctx := setupTest(t)
			mockConfig.SetInTest("infrastructure_mode", tt.mode)

			hostTags := Get(ctx, false, mockConfig)
			assert.Contains(t, hostTags.System, tt.want)
		})
	}
}

func TestGetWithEUDM(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("infrastructure_mode", "end_user_device")

	original := collectEUDMTagsFunc
	t.Cleanup(func() { collectEUDMTagsFunc = original })
	collectEUDMTagsFunc = func() []string {
		return []string{
			"os_name:darwin",
			"os_version:23.5.0",
			"cpu_model:Apple_M1_Pro",
			"total_memory_gb:16",
			"device_model:MacBookPro18,3",
			"hostid:TEST123",
		}
	}

	hostTags := Get(ctx, false, mockConfig)
	assert.Contains(t, hostTags.System, "infra_mode:end_user_device")
	assert.Contains(t, hostTags.System, "os_name:darwin")
	assert.Contains(t, hostTags.System, "os_version:23.5.0")
	assert.Contains(t, hostTags.System, "cpu_model:Apple_M1_Pro")
	assert.Contains(t, hostTags.System, "total_memory_gb:16")
	assert.Contains(t, hostTags.System, "device_model:MacBookPro18,3")
	assert.Contains(t, hostTags.System, "hostid:TEST123")
}

func TestEUDMTagsOnUnsupportedOS(t *testing.T) {
	// collectEUDMHardwareTags should return nil on non-darwin/windows so the
	// only EUDM tag emitted on Linux is the infra_mode marker.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("test asserts behavior on non-darwin/non-windows hosts")
	}
	assert.Nil(t, collectEUDMHardwareTags())

	tags := getEUDMTags()
	assert.Equal(t, []string{"infra_mode:end_user_device"}, tags)
}

func TestBytesToGB(t *testing.T) {
	assert.Equal(t, uint64(16), bytesToGB(16*1024*1024*1024))
	assert.Equal(t, uint64(0), bytesToGB(0))
	assert.Equal(t, uint64(1), bytesToGB(1024*1024*1024))
	// 15.9 GiB rounds to 16
	assert.Equal(t, uint64(16), bytesToGB(15*1024*1024*1024+900*1024*1024))
}

func TestSanitizeEUDMTagValue(t *testing.T) {
	assert.Equal(t, "Apple_M1_Pro", sanitizeEUDMTagValue("Apple M1 Pro"))
	assert.Equal(t, "MacBookPro18,3", sanitizeEUDMTagValue("MacBookPro18,3"))
	assert.Equal(t, "trim_me", sanitizeEUDMTagValue("  trim me  "))
}

func TestGetProvidersDefinitionsSkipsKubernetesNodeTagsOnCLCRunner(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	env.SetFeatures(t, env.Kubernetes)

	mockConfig.SetInTest("clc_runner_enabled", true)
	mockConfig.SetInTest("config_providers", []map[string]interface{}{{"name": "clusterchecks"}})

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasKubernetesNodeTags := providers["kubernetes"]
	assert.False(t, hasKubernetesNodeTags, "kubernetes node-tags provider should be skipped on Cluster Checks Runners, which have no reachable local Kubelet")

	_, hasClusterAgentTags := providers["kubernetes_cluster_agent_tags"]
	assert.True(t, hasClusterAgentTags, "kubernetes_cluster_agent_tags provider should still be registered on Cluster Checks Runners")
}

func TestGetProvidersDefinitionsIncludesKubernetesNodeTagsOnNodeAgent(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	env.SetFeatures(t, env.Kubernetes)

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasKubernetesNodeTags := providers["kubernetes"]
	assert.True(t, hasKubernetesNodeTags, "kubernetes node-tags provider should be registered on a regular node Agent")
}

func TestGetProvidersDefinitionsSkipsEC2AndGCEWhenRunningOnAzure(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_ec2_tags", true)
	mockConfig.SetInTest("collect_ec2_instance_info", true)
	setupCloudProvider(t, azure.CloudProviderName)

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasGCE := providers["gce"]
	assert.False(t, hasGCE, "gce provider should be skipped when the host is running on Azure")
	_, hasEC2 := providers["ec2"]
	assert.False(t, hasEC2, "ec2 provider should be skipped when the host is running on Azure")
	_, hasEC2InstanceInfo := providers["ec2_instance_info"]
	assert.False(t, hasEC2InstanceInfo, "ec2_instance_info provider should be skipped when the host is running on Azure")
}

func TestGetProvidersDefinitionsSkipsEC2AndGCEWhenRunningOnOracle(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_ec2_tags", true)
	mockConfig.SetInTest("collect_ec2_instance_info", true)
	setupCloudProvider(t, oracle.CloudProviderName)

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasGCE := providers["gce"]
	assert.False(t, hasGCE, "gce provider should be skipped when the host is running on Oracle Cloud")
	_, hasEC2 := providers["ec2"]
	assert.False(t, hasEC2, "ec2 provider should be skipped when the host is running on Oracle Cloud")
	_, hasEC2InstanceInfo := providers["ec2_instance_info"]
	assert.False(t, hasEC2InstanceInfo, "ec2_instance_info provider should be skipped when the host is running on Oracle Cloud")
}

func TestGetProvidersDefinitionsKeepsMatchingProviderWhenRunningOnGCE(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_ec2_tags", true)
	mockConfig.SetInTest("collect_ec2_instance_info", true)
	setupCloudProvider(t, gce.CloudProviderName)

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasGCE := providers["gce"]
	assert.True(t, hasGCE, "gce provider should still be registered when the host is running on GCE")
	_, hasEC2 := providers["ec2"]
	assert.False(t, hasEC2, "ec2 provider should be skipped when the host is running on GCE")
	_, hasEC2InstanceInfo := providers["ec2_instance_info"]
	assert.False(t, hasEC2InstanceInfo, "ec2_instance_info provider should be skipped when the host is running on GCE")
}

func TestGetProvidersDefinitionsKeepsMatchingProviderWhenRunningOnEC2(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_ec2_tags", true)
	mockConfig.SetInTest("collect_ec2_instance_info", true)
	setupCloudProvider(t, ec2.CloudProviderName)

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasGCE := providers["gce"]
	assert.False(t, hasGCE, "gce provider should be skipped when the host is running on EC2")
	_, hasEC2 := providers["ec2"]
	assert.True(t, hasEC2, "ec2 provider should still be registered when the host is running on EC2")
	_, hasEC2InstanceInfo := providers["ec2_instance_info"]
	assert.True(t, hasEC2InstanceInfo, "ec2_instance_info provider should still be registered when the host is running on EC2")
}

func TestGetProvidersDefinitionsKeepsAllProvidersWhenNoCloudProviderDetected(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_ec2_tags", true)
	mockConfig.SetInTest("collect_ec2_instance_info", true)
	setupCloudProvider(t, "")

	providers := getProvidersDefinitions(ctx, mockConfig)
	_, hasGCE := providers["gce"]
	assert.True(t, hasGCE, "gce provider should still be registered when no cloud provider is detected")
	_, hasEC2 := providers["ec2"]
	assert.True(t, hasEC2, "ec2 provider should still be registered when no cloud provider is detected")
	_, hasEC2InstanceInfo := providers["ec2_instance_info"]
	assert.True(t, hasEC2InstanceInfo, "ec2_instance_info provider should still be registered when no cloud provider is detected")
}

func TestHostTagsCache(t *testing.T) {
	mockConfig, ctx := setupTest(t)
	mockConfig.SetInTest("collect_gce_tags", false)

	fooTags := []string{"foo1:value1"}
	var fooErr error
	nbCall := 0

	getProvidersDefinitionsFunc = func(context.Context, model.Reader) map[string]*providerDef {
		return map[string]*providerDef{
			"foo": {
				retries: 2,
				getTags: func(_ context.Context) ([]string, error) {
					nbCall++
					return fooTags, fooErr
				},
			},
		}
	}

	// First run, all good
	hostTags := Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
	assert.Equal(t, 1, nbCall)

	// Second run, provider all fails, we should get cached data
	fooErr = errors.New("fooerr")
	nbCall = 0

	hostTags = Get(ctx, false, mockConfig)
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, []string{"foo1:value1"}, hostTags.System)
	assert.Equal(t, 2, nbCall)
}

func TestEUDMSystemInfoTags(t *testing.T) {
	for _, tc := range []struct {
		name string
		info *systeminfo.SystemInfo
		want []string
	}{
		{name: "unavailable"},
		{name: "empty", info: &systeminfo.SystemInfo{}},
		{name: "model only", info: &systeminfo.SystemInfo{Identifier: "MacBookPro18,3"}, want: []string{"device_model:MacBookPro18,3"}},
		{name: "serial without model", info: &systeminfo.SystemInfo{SerialNumber: "TEST123"}, want: []string{"hostid:TEST123"}},
		{name: "model and serial", info: &systeminfo.SystemInfo{Identifier: "MacBookPro18,3", SerialNumber: "TEST123"}, want: []string{"device_model:MacBookPro18,3", "hostid:TEST123"}},
		{name: "whitespace serial", info: &systeminfo.SystemInfo{SerialNumber: " \t\r\n "}},
		{name: "sanitize serial", info: &systeminfo.SystemInfo{SerialNumber: "  TEST 123\tABC  "}, want: []string{"hostid:TEST_123_ABC"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, eudmSystemInfoTags(tc.info))
		})
	}
}
