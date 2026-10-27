// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustername

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestGetClusterName(t *testing.T) {
	ctx := context.Background()
	mockConfig := configmock.New(t)
	// Disable cloud provider metadata detection to avoid real HTTP calls to
	// EC2/GCE/Azure metadata endpoints.
	mockConfig.SetInTest("cloud_provider_metadata", []string{})
	env.SetFeatures(t, env.Kubernetes)
	data := newClusterNameData()

	testClusterName := "laika"
	mockConfig.SetInTest("cluster_name", testClusterName)
	defer mockConfig.SetInTest("cluster_name", nil)

	assert.Equal(t, testClusterName, getClusterName(ctx, data, "hostname"))

	// Test caching and reset
	newClusterName := "youri"
	mockConfig.SetInTest("cluster_name", newClusterName)
	assert.Equal(t, testClusterName, getClusterName(ctx, data, "hostname"))
	freshData := newClusterNameData()
	assert.Equal(t, newClusterName, getClusterName(ctx, freshData, "hostname"))

	dotClusterName := "aclusternamewitha.dot"
	mockConfig.SetInTest("cluster_name", dotClusterName)
	data = newClusterNameData()
	assert.Equal(t, dotClusterName, getClusterName(ctx, data, "hostname"))

	dotsClusterName := "a.cluster.name.with.dots"
	mockConfig.SetInTest("cluster_name", dotsClusterName)
	data = newClusterNameData()
	assert.Equal(t, dotsClusterName, getClusterName(ctx, data, "hostname"))

	startsWithNumberName := "1cluster"
	mockConfig.SetInTest("cluster_name", startsWithNumberName)
	data = newClusterNameData()
	assert.Equal(t, startsWithNumberName, getClusterName(ctx, data, "hostname"))

	underscoreClusterName := "cluster_with_underscore"
	mockConfig.SetInTest("cluster_name", underscoreClusterName)
	data = newClusterNameData()
	assert.Equal(t, MakeClusterNameRFC1123Compliant(underscoreClusterName), getClusterName(ctx, data, "hostname"))

	digitLeadingDottedClusterName := "1a.cluster.name"
	mockConfig.SetInTest("cluster_name", digitLeadingDottedClusterName)
	data = newClusterNameData()
	assert.Equal(t, digitLeadingDottedClusterName, getClusterName(ctx, data, "hostname"))

	// Test invalid cluster names
	for _, invalidClusterName := range []string{
		"Capital",
		"with_dot._underscore",
		"toolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoolongtoo",
		"a..a",
		"mx.gmail.com.",
	} {
		mockConfig.SetInTest("cluster_name", invalidClusterName)
		freshData = newClusterNameData()
		assert.Equal(t, "", getClusterName(ctx, freshData, "hostname"))
	}

	mockConfig.SetInTest("cluster_name", "")

	// Test lowercase
	wantedClustername := "foo"
	discoveredClustername := "FoO"
	dummyFunc := func(context.Context) (string, error) { return discoveredClustername, nil }
	setProviderCatalog(map[string]Provider{"dummyProvider": dummyFunc})
	assert.Equal(t, wantedClustername, getClusterName(ctx, newClusterNameData(), "hostname"))
}

// TestGetClusterNameCLCRunner ensures that a Cluster Checks Runner (which
// never has a locally-reachable kubelet) still honors a cluster name provided
// via config, without needing the node-label based auto discovery.
func TestGetClusterNameCLCRunner(t *testing.T) {
	ctx := context.Background()
	mockConfig := configmock.New(t)
	env.SetFeatures(t, env.Kubernetes)

	mockConfig.SetInTest("clc_runner_enabled", true)
	mockConfig.SetInTest("config_providers", []map[string]interface{}{{"name": "clusterchecks"}})

	testClusterName := "laika"
	mockConfig.SetInTest("cluster_name", testClusterName)

	assert.Equal(t, testClusterName, getClusterName(ctx, newClusterNameData(), "hostname"))
}

// TestGetClusterNameOTelResourceAttributes verifies that otel-agent falls
// back on the k8s.cluster.name attribute of its telemetry resource, from
// service.telemetry.resource in the collector config or else from
// OTEL_RESOURCE_ATTRIBUTES, in standalone mode only, and that other flavors
// ignore it.
func TestGetClusterNameOTelResourceAttributes(t *testing.T) {
	tests := []struct {
		name   string
		flavor string
		// connected runs otel-agent in connected mode rather than standalone.
		connected         bool
		configClusterName string
		cloudClusterName  string
		// nodeLabelClusterName is what an EKS or AKS node label holds: it only
		// applies when no cluster name was found before.
		nodeLabelClusterName string
		// telemetryResourceClusterName is what otel-agent read from
		// service.telemetry.resource in the collector config.
		telemetryResourceClusterName string
		resourceAttributes           string
		want                         string
	}{
		{
			name:               "otel-agent reads k8s.cluster.name",
			flavor:             flavor.OTelAgent,
			resourceAttributes: "k8s.pod.name=pod,k8s.cluster.name=laika,k8s.node.name=node",
			want:               "laika",
		},
		{
			name:               "the OTel Helm chart default is made RFC 1123 compliant",
			flavor:             flavor.OTelAgent,
			resourceAttributes: "k8s.cluster.name=unknown_k8s_cluster",
			want:               "unknown-k8s-cluster",
		},
		{
			name:               "parsed as the OTel SDK does",
			flavor:             flavor.OTelAgent,
			resourceAttributes: " k8s.cluster.name = first , k8s.cluster.name = My%2ECluster ",
			want:               "my.cluster",
		},
		{
			name:               "an invalid name is ignored",
			flavor:             flavor.OTelAgent,
			resourceAttributes: "k8s.cluster.name=my cluster",
			want:               "",
		},
		{
			name:               "cluster_name takes precedence",
			flavor:             flavor.OTelAgent,
			configClusterName:  "youri",
			resourceAttributes: "k8s.cluster.name=laika",
			want:               "youri",
		},
		{
			name:               "a cloud provider takes precedence over the chart's placeholder",
			flavor:             flavor.OTelAgent,
			cloudClusterName:   "eks-cluster",
			resourceAttributes: "k8s.cluster.name=unknown_k8s_cluster",
			want:               "eks-cluster",
		},
		{
			name:                 "a node label takes precedence over the chart's placeholder",
			flavor:               flavor.OTelAgent,
			nodeLabelClusterName: "eks-cluster",
			resourceAttributes:   "k8s.cluster.name=unknown_k8s_cluster",
			want:                 "eks-cluster",
		},
		{
			name:                         "service.telemetry.resource alone",
			flavor:                       flavor.OTelAgent,
			telemetryResourceClusterName: "From-Config",
			want:                         "from-config",
		},
		{
			name:                         "service.telemetry.resource takes precedence over OTEL_RESOURCE_ATTRIBUTES",
			flavor:                       flavor.OTelAgent,
			telemetryResourceClusterName: "from-config",
			resourceAttributes:           "k8s.cluster.name=unknown_k8s_cluster",
			want:                         "from-config",
		},
		{
			name:                         "a cloud provider takes precedence over service.telemetry.resource",
			flavor:                       flavor.OTelAgent,
			cloudClusterName:             "eks-cluster",
			telemetryResourceClusterName: "from-config",
			want:                         "eks-cluster",
		},
		{
			name:               "unset",
			flavor:             flavor.OTelAgent,
			resourceAttributes: "k8s.node.name=node",
			want:               "",
		},
		{
			name:                         "otel-agent in connected mode ignores it",
			flavor:                       flavor.OTelAgent,
			connected:                    true,
			telemetryResourceClusterName: "from-config",
			resourceAttributes:           "k8s.cluster.name=laika",
			want:                         "",
		},
		{
			name:               "other flavors ignore it",
			flavor:             flavor.DefaultAgent,
			resourceAttributes: "k8s.cluster.name=laika",
			want:               "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("cloud_provider_metadata", []string{})
			mockConfig.SetInTest("cluster_name", tt.configClusterName)
			mockConfig.SetInTest("otelcollector.standalone.telemetry_resource_cluster_name", tt.telemetryResourceClusterName)
			mockConfig.SetInTest("otel_standalone", !tt.connected)
			env.SetFeatures(t, env.Kubernetes)
			flavor.SetTestFlavor(t, tt.flavor)
			catalog := map[string]Provider{}
			if tt.cloudClusterName != "" {
				catalog["dummyProvider"] = func(context.Context) (string, error) { return tt.cloudClusterName, nil }
			}
			setProviderCatalog(catalog)
			stubNodeClusterNameLabel(t, func(_ context.Context, clusterName string) (string, error) {
				if clusterName == "" {
					return tt.nodeLabelClusterName, nil
				}
				return clusterName, nil
			})
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tt.resourceAttributes)

			assert.Equal(t, tt.want, getClusterName(context.Background(), newClusterNameData(), "hostname"))
		})
	}
}

// TestGetClusterNameNodeLabelsWithoutKubelet verifies that the kubelet-backed
// node-label discovery is skipped when otel-agent's configuration leaves it
// without kubelet access, and still runs otherwise.
func TestGetClusterNameNodeLabelsWithoutKubelet(t *testing.T) {
	tests := []struct {
		name                string
		flavor              string
		useKubeletCollector bool
		wantNodeLabels      bool
	}{
		{name: "otel-agent with the nodefilter collector", flavor: flavor.OTelAgent, useKubeletCollector: false, wantNodeLabels: false},
		{name: "otel-agent with the kubelet collector", flavor: flavor.OTelAgent, useKubeletCollector: true, wantNodeLabels: true},
		{name: "other flavors", flavor: flavor.DefaultAgent, useKubeletCollector: false, wantNodeLabels: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("cloud_provider_metadata", []string{})
			mockConfig.SetInTest("otel_standalone", true)
			mockConfig.SetInTest("otelcollector.standalone.use_kubelet_collector", tt.useKubeletCollector)
			env.SetFeatures(t, env.Kubernetes)
			flavor.SetTestFlavor(t, tt.flavor)
			setProviderCatalog(map[string]Provider{})
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "k8s.cluster.name=laika")

			calledNodeLabels := false
			stubNodeClusterNameLabel(t, func(context.Context, string) (string, error) {
				calledNodeLabels = true
				return "", errors.New("no kubelet")
			})

			clusterName := getClusterName(context.Background(), newClusterNameData(), "hostname")
			assert.Equal(t, tt.wantNodeLabels, calledNodeLabels)
			if tt.flavor == flavor.OTelAgent {
				assert.Equal(t, "laika", clusterName)
			}
		})
	}
}

func TestGetClusterID(t *testing.T) {
	clearClusterIDCache(t)
	// missing env
	cid, err := GetClusterID()
	assert.Empty(t, cid)
	assert.NotNil(t, err)

	// too short
	t.Setenv(clusterIDEnv, "foo")
	cid, err = GetClusterID()
	assert.Empty(t, cid)
	assert.NotNil(t, err)

	// too long
	t.Setenv(clusterIDEnv, "d801b2b1-4811-11ea-8618-121d4d0938a44444444")
	cid, err = GetClusterID()
	assert.Empty(t, cid)
	assert.NotNil(t, err)

	// just right
	testID := "d801b2b1-4811-11ea-8618-121d4d0938a3"
	t.Setenv(clusterIDEnv, testID)
	cid, err = GetClusterID()
	assert.Equal(t, testID, cid)
	assert.Nil(t, err)
}

func TestGetClusterIDClusterAgent(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.ClusterAgent)
	// The env variable must not be used by the Cluster Agent.
	t.Setenv(clusterIDEnv, "d801b2b1-4811-11ea-8618-121d4d0938a3")
	previousLookup := getClusterAgentClusterID
	t.Cleanup(func() { getClusterAgentClusterID = previousLookup })

	lookupError := errors.New("Kubernetes unavailable")
	for name, tc := range map[string]struct {
		id  string
		err error
	}{
		"success": {id: "226430c6-5e57-11ea-91d5-42010a8400c6"},
		"failure": {err: lookupError},
	} {
		t.Run(name, func(t *testing.T) {
			clearClusterIDCache(t)
			getClusterAgentClusterID = func() (string, error) { return tc.id, tc.err }

			id, err := GetClusterID()
			require.ErrorIs(t, err, tc.err)
			assert.Equal(t, tc.id, id)
		})
	}
}

// stubNodeClusterNameLabel replaces the node-label discovery with stub for the
// duration of the test.
func stubNodeClusterNameLabel(t *testing.T, stub func(context.Context, string) (string, error)) {
	t.Helper()
	previous := nodeClusterNameLabel
	t.Cleanup(func() { nodeClusterNameLabel = previous })
	nodeClusterNameLabel = stub
}

func clearClusterIDCache(t *testing.T) {
	t.Helper()
	key := cache.BuildAgentKey(constants.ClusterIDCacheKey)
	cache.Cache.Delete(key)
	t.Cleanup(func() { cache.Cache.Delete(key) })
}
