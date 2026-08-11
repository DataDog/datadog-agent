// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"strconv"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/configmapdata"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// allowConfigMapData opts the given ConfigMaps of the test cluster into full data collection, as
// remote config would, and restores the empty allow-list afterwards.
func allowConfigMapData(t *testing.T, ctx *processors.K8sProcessorContext, clusterID string, namespacedNames ...[2]string) {
	setConfigMapEntries(t, ctx, entriesFor(clusterID, true, 1000, namespacedNames...)...)
}

// denyConfigMapData records an explicit opt-out for the given ConfigMaps, which is how remote config
// carries one: the entry stays, with a newer timestamp and data collection turned off.
func denyConfigMapData(t *testing.T, ctx *processors.K8sProcessorContext, clusterID string, namespacedNames ...[2]string) {
	setConfigMapEntries(t, ctx, entriesFor(clusterID, false, 2000, namespacedNames...)...)
}

func entriesFor(clusterID string, dataCollected bool, timestamp int64, namespacedNames ...[2]string) []configmapdata.Entry {
	entries := make([]configmapdata.Entry, 0, len(namespacedNames))
	for _, nn := range namespacedNames {
		entries = append(entries, configmapdata.Entry{
			ClusterID:     clusterID,
			Namespace:     nn[0],
			Name:          nn[1],
			Timestamp:     timestamp,
			DataCollected: dataCollected,
		})
	}
	return entries
}

// setConfigMapEntries replaces the allow-list and re-snapshots it onto the context, the way a tick
// following a remote config update would.
func setConfigMapEntries(t *testing.T, ctx *processors.K8sProcessorContext, entries ...configmapdata.Entry) {
	configmapdata.Get().Replace(entries)
	t.Cleanup(func() { configmapdata.Get().Replace(nil) })

	if ctx != nil {
		ctx.ConfigMapAllowSet = configmapdata.Get().Snapshot(ctx.ClusterID)
	}
}

func createTestConfigMap(name string) *corev1.ConfigMap {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "test-namespace",
			UID:               "test-configmap-uid-123",
			ResourceVersion:   "1234",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		BinaryData: map[string][]byte{
			"bin-key": []byte("binary-value"),
		},
	}
	cm.ManagedFields = []metav1.ManagedFieldsEntry{
		{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
	}
	return cm
}

func newConfigMapProcessorContext(cfg *orchestratorconfig.OrchestratorConfig) *processors.K8sProcessorContext {
	return &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "ConfigMap",
			APIVersion:       "v1",
			NodeType:         orchestrator.K8sConfigMap,
		},
		APIClient: &apiserver.APIClient{Cl: fake.NewClientset()},
		HostName:  "test-host",
	}
}

func TestConfigMapHandlers_ResourceList(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm1 := createTestConfigMap("cm-1")
	cm2 := createTestConfigMap("cm-2")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	resources := handlers.ResourceList(ctx, []*corev1.ConfigMap{cm1, cm2})

	assert.Len(t, resources, 2)

	r1, ok := resources[0].(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.Equal(t, "cm-1", r1.Name)
	assert.Same(t, cm1, r1)

	r2, ok := resources[1].(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.Equal(t, "cm-2", r2.Name)
	assert.Same(t, cm2, r2)
}

func TestConfigMapHandlers_ResourceUID(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	expectedUID := types.UID("test-uid-456")
	cm.UID = expectedUID

	uid := handlers.ResourceUID(nil, cm)
	assert.Equal(t, expectedUID, uid)
}

func TestConfigMapHandlers_ResourceVersion(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "v999"

	version := handlers.ResourceVersion(nil, cm, nil)
	assert.Equal(t, "v999", version)
}

func TestConfigMapHandlers_ResourceVersionFromRaw(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "v42"

	version := handlers.ResourceVersionFromRaw(nil, cm)
	assert.Equal(t, "v42", version)
}

func TestConfigMapHandlers_BeforeMarshalling(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	skip := handlers.BeforeMarshalling(ctx, cm, nil)

	assert.False(t, skip)
	assert.Equal(t, "ConfigMap", cm.Kind)
	assert.Equal(t, "v1", cm.APIVersion)
}

func TestConfigMapHandlers_AfterMarshalling(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	testYAML := []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`)
	skip := handlers.AfterMarshalling(ctx, cm, nil, testYAML)
	assert.False(t, skip)
}

func TestConfigMapHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	cm.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	handlers.ScrubBeforeExtraction(ctx, cm)

	assert.Equal(t, "-", cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", cm.Labels["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "my-annotation", cm.Annotations["annotation"])
	assert.Equal(t, "my-app", cm.Labels["app"])
}

func TestConfigMapHandlers_ScrubBeforeMarshalling(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	assert.NotEmpty(t, cm.Data)
	assert.NotEmpty(t, cm.BinaryData)
	assert.NotEmpty(t, cm.ManagedFields)

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	handlers.ScrubBeforeMarshalling(ctx, cm)

	assert.Nil(t, cm.Data)
	assert.Nil(t, cm.BinaryData)
	assert.Nil(t, cm.ManagedFields)
}

func TestConfigMapHandlers_ScrubBeforeMarshallingDataCollected(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	allowConfigMapData(t, ctx, ctx.ClusterID, [2]string{cm.Namespace, cm.Name})

	handlers.ScrubBeforeMarshalling(ctx, cm)

	assert.Equal(t, map[string]string{"key1": "value1", "key2": "value2"}, cm.Data)
	assert.NotEmpty(t, cm.BinaryData)
	assert.NotEmpty(t, cm.ManagedFields, "managedFields is kept when data is collected")
}

func TestConfigMapHandlers_ScrubBeforeMarshallingScrubsCollectedData(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.Data = map[string]string{
		"app.conf": "# a comment\n\nlisten = 8080\napi_key: abcdef0123456789abcdef0123456789\n",
	}

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	allowConfigMapData(t, ctx, ctx.ClusterID, [2]string{cm.Namespace, cm.Name})

	handlers.ScrubBeforeMarshalling(ctx, cm)

	scrubbed := cm.Data["app.conf"]
	assert.NotContains(t, scrubbed, "abcdef0123456789abcdef0123456789")
	// Scrubbing must not rewrite the parts it does not redact: line-oriented scrubbing would drop
	// the comment and collapse the blank line.
	assert.Contains(t, scrubbed, "# a comment")
	assert.Contains(t, scrubbed, "\n\n")
	assert.Contains(t, scrubbed, "listen = 8080")
}

func TestConfigMapHandlers_ScrubBeforeMarshallingOtherCluster(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	// Same namespace and name, different cluster: remote config configs are org-scoped, so this must
	// not opt the local ConfigMap in.
	allowConfigMapData(t, ctx, "other-cluster-id", [2]string{cm.Namespace, cm.Name})

	handlers.ScrubBeforeMarshalling(ctx, cm)

	assert.Nil(t, cm.Data)
	assert.Nil(t, cm.BinaryData)
	assert.Nil(t, cm.ManagedFields)
}

func TestConfigMapHandlers_ResourceVersionDataCollected(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "1234"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	allowConfigMapData(t, ctx, ctx.ClusterID, [2]string{cm.Namespace, cm.Name})

	version := handlers.ResourceVersion(ctx, cm, nil)

	// The version the caches compare must differ from the etcd one so that the opt-in looks like a
	// change, and it must stay a base-10 uint64 because the backend parses it as one.
	assert.Equal(t, handlers.ResourceVersionFromRaw(ctx, cm), version, "the cached and emitted versions must agree")
	assert.NotEqual(t, cm.ResourceVersion, version)

	_, err := strconv.ParseUint(version, 10, 64)
	assert.NoError(t, err)
}

func TestConfigMapHandlers_ResourceVersionFlips(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "1234"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	nn := [2]string{cm.Namespace, cm.Name}

	// The backend deduplicates writes on the manifest hash and the resource version together, so
	// every flip has to report a version none of the earlier states reported. The object itself never
	// changes here: only the decision does.
	versions := map[string]string{}
	record := func(state string, entries ...configmapdata.Entry) {
		setConfigMapEntries(t, ctx, entries...)
		v := handlers.ResourceVersion(ctx, cm, nil)
		assert.Equal(t, handlers.ResourceVersionFromRaw(ctx, cm), v, "%s: the cached and emitted versions must agree", state)
		for earlierState, earlier := range versions {
			assert.NotEqual(t, earlier, v, "%s reported the same version as %s", state, earlierState)
		}
		versions[state] = v
	}

	record("never mentioned")
	record("opted in", configmapdata.Entry{ClusterID: ctx.ClusterID, Namespace: nn[0], Name: nn[1], Timestamp: 1000, DataCollected: true})
	record("opted out", configmapdata.Entry{ClusterID: ctx.ClusterID, Namespace: nn[0], Name: nn[1], Timestamp: 2000})
	record("opted back in", configmapdata.Entry{ClusterID: ctx.ClusterID, Namespace: nn[0], Name: nn[1], Timestamp: 3000, DataCollected: true})

	assert.Equal(t, "1234", versions["never mentioned"], "a ConfigMap nobody asked about reports the etcd version untouched")
}

func TestConfigMapHandlers_ResourceVersionOptedOut(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "1234"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	denyConfigMapData(t, ctx, ctx.ClusterID, [2]string{cm.Namespace, cm.Name})

	// An opt-out strips the data like a ConfigMap that was never opted in, but it must not report the
	// etcd version: that pair of manifest hash and resource version was already accepted by the
	// backend before the opt-in, so the write removing the data would be deduped away.
	assert.False(t, isDataCollected(ctx, cm))
	assert.NotEqual(t, "1234", handlers.ResourceVersion(ctx, cm, nil))
	assert.Equal(t, handlers.ResourceVersionFromRaw(ctx, cm), handlers.ResourceVersion(ctx, cm, nil))
}

func TestConfigMapHandlers_ResourceVersionNotCollected(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "1234"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	assert.Equal(t, "1234", handlers.ResourceVersion(ctx, cm, nil))
	assert.Equal(t, "1234", handlers.ResourceVersionFromRaw(ctx, cm))
}

func TestConfigMapHandlers_ResourceVersionOpaqueDataCollected(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	// A resource version is opaque per the Kubernetes API contract, so it is mixed in as bytes rather
	// than parsed. An opted-in ConfigMap with a non-numeric version still reports something the
	// backend can parse, and still reports something new.
	cm.ResourceVersion = "not-a-number"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	allowConfigMapData(t, ctx, ctx.ClusterID, [2]string{cm.Namespace, cm.Name})

	version := handlers.ResourceVersion(ctx, cm, nil)
	assert.Equal(t, handlers.ResourceVersionFromRaw(ctx, cm), version)
	assert.NotEqual(t, "not-a-number", version)

	_, err := strconv.ParseUint(version, 10, 64)
	assert.NoError(t, err)
}

func TestConfigMapHandlers_ResourceListSnapshotsTheAllowList(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("cm-1")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)
	allowConfigMapData(t, nil, ctx.ClusterID, [2]string{cm.Namespace, cm.Name})

	handlers.ResourceList(ctx, []*corev1.ConfigMap{cm})
	assert.True(t, ctx.ConfigMapAllowSet.IsAllowed(cm.Namespace, cm.Name))

	// A remote config update landing mid-tick must not change the answer for the rest of the tick.
	configmapdata.Get().Replace(nil)
	assert.True(t, ctx.ConfigMapAllowSet.IsAllowed(cm.Namespace, cm.Name))
}

func TestConfigMapHandlers_CloneResource(t *testing.T) {
	handlers := NewConfigMapHandlers()

	original := createTestConfigMap("test-cm")
	cloned := handlers.CloneResource(original)

	clonedTyped, ok := cloned.(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.NotSame(t, original, clonedTyped)
	assert.Equal(t, original, clonedTyped)

	// Mutating the clone must not affect the original (informer cache protection).
	clonedTyped.Data = nil
	clonedTyped.BinaryData = nil
	clonedTyped.ManagedFields = nil
	assert.NotEmpty(t, original.Data)
	assert.NotEmpty(t, original.BinaryData)
	assert.NotEmpty(t, original.ManagedFields)
}

func TestConfigMapHandlers_BuildManifestMessageBody(t *testing.T) {
	handlers := NewConfigMapHandlers()

	manifest1 := &model.Manifest{
		Uid:             "test-uid-1",
		ResourceVersion: "1203",
		Type:            int32(orchestrator.K8sConfigMap),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-1"}}`),
	}
	manifest2 := &model.Manifest{
		Uid:             "test-uid-2",
		ResourceVersion: "5678",
		Type:            int32(orchestrator.K8sConfigMap),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-2"}}`),
	}

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newConfigMapProcessorContext(cfg)

	messageBody := handlers.BuildManifestMessageBody(ctx, []interface{}{manifest1, manifest2}, 2)

	collectorMsg, ok := messageBody.(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Manifests, 2)
}

func TestConfigMapProcessor_Process(t *testing.T) {
	cm1 := createTestConfigMap("cm-1")
	cm1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	cm1.ResourceVersion = "1203"

	cm2 := createTestConfigMap("cm-2")
	cm2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	cm2.ResourceVersion = "1303"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newConfigMapProcessorContext(cfg)

	processor := processors.NewProcessor(NewConfigMapHandlers())
	result, listed, processed := processor.Process(ctx, []*corev1.ConfigMap{cm1, cm2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)

	// ConfigMap is manifest-only: metadata messages are built but nil.
	assert.Len(t, result.MetadataMessages, 1)
	assert.Nil(t, result.MetadataMessages[0])
	assert.Len(t, result.ManifestMessages, 1)

	collectorMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, "test-host", collectorMsg.HostName)
	assert.Len(t, collectorMsg.Manifests, 2)

	manifest1 := collectorMsg.Manifests[0]
	assert.Equal(t, string(cm1.UID), manifest1.Uid)
	assert.Equal(t, cm1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(orchestrator.K8sConfigMap), manifest1.Type)
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Data and BinaryData must be absent from the emitted manifest.
	var parsed map[string]interface{}
	err := yaml.Unmarshal(manifest1.Content, &parsed)
	assert.NoError(t, err)
	assert.NotContains(t, parsed, "data", "data must be stripped before marshalling")
	assert.NotContains(t, parsed, "binaryData", "binaryData must be stripped before marshalling")

	metadata, ok := parsed["metadata"].(map[string]interface{})
	assert.True(t, ok)
	assert.NotContains(t, metadata, "managedFields", "managedFields must be stripped before marshalling")
	assert.Equal(t, "cm-1", metadata["name"])
	assert.Equal(t, string(cm1.UID), metadata["uid"])

	// Verify the informer cache is unaffected: the originals still have their data.
	assert.NotEmpty(t, cm1.Data)
	assert.NotEmpty(t, cm1.BinaryData)
}

func TestConfigMapProcessor_ProcessDataCollected(t *testing.T) {
	// The resource cache is process-global and keyed on UID, so use UIDs no other test has seen.
	cm1 := createTestConfigMap("cm-1")
	cm1.UID = types.UID("a1b2c3d4-0749-11e8-a2b8-000c29dea401")
	cm1.ResourceVersion = "1203"

	cm2 := createTestConfigMap("cm-2")
	cm2.UID = types.UID("a1b2c3d4-0749-11e8-a2b8-000c29dea402")
	cm2.ResourceVersion = "1303"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newConfigMapProcessorContext(cfg)
	// Only cm-1 is opted in. The snapshot is taken by ResourceList, so it is not set here.
	allowConfigMapData(t, nil, ctx.ClusterID, [2]string{cm1.Namespace, cm1.Name})

	processor := processors.NewProcessor(NewConfigMapHandlers())
	result, _, processed := processor.Process(ctx, []*corev1.ConfigMap{cm1, cm2})
	assert.Equal(t, 2, processed)

	collectorMsg := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.Len(t, collectorMsg.Manifests, 2)

	manifest1, manifest2 := collectorMsg.Manifests[0], collectorMsg.Manifests[1]

	// The opted-in ConfigMap keeps its data and carries a derived version, so that neither the agent
	// cache nor the backend caches dedupe the opt-in away.
	_, err := strconv.ParseUint(manifest1.ResourceVersion, 10, 64)
	assert.NoError(t, err)
	assert.NotEqual(t, "1203", manifest1.ResourceVersion)

	var parsed1 map[string]interface{}
	assert.NoError(t, yaml.Unmarshal(manifest1.Content, &parsed1))
	assert.Contains(t, parsed1, "data")
	assert.Contains(t, parsed1, "binaryData")

	metadata1 := parsed1["metadata"].(map[string]interface{})
	assert.Contains(t, metadata1, "managedFields", "managedFields is kept when data is collected")
	// The derived version applies to the payload envelope only: the manifest body keeps the real etcd
	// version.
	assert.Equal(t, "1203", metadata1["resourceVersion"])

	// The ConfigMap that was not opted in is unaffected.
	assert.Equal(t, "1303", manifest2.ResourceVersion)

	var parsed2 map[string]interface{}
	assert.NoError(t, yaml.Unmarshal(manifest2.Content, &parsed2))
	assert.NotContains(t, parsed2, "data")
	assert.NotContains(t, parsed2, "binaryData")
}
