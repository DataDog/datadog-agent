// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var manifestToSend []*model.CollectorManifest

func TestOrchestratorManifestBuffer(t *testing.T) {
	manifestToSend = []*model.CollectorManifest{}
	mb := getManifestBuffer(t)
	mb.Start(getSender(t))

	var body model.MessageBody = &model.CollectorManifest{
		Manifests: []*model.Manifest{
			{
				Type: int32(1),
				Tags: []string{"tag_for:type1"},
			},
			{
				Type: int32(2),
			},
			{
				Type: int32(3),
			},
			{
				Type: int32(4),
			},
			{
				Type: int32(5),
			},
		},
		Tags:        []string{"dropped:tag"},
		ClusterName: "dropped-cluster",
	}
	BufferManifestProcessResult([]model.MessageBody{body}, mb)
	mb.Stop()

	// Buffer size is 2, as we have 5 manifests, the buffer needs to be flushed 3 times
	require.Len(t, manifestToSend, 3)

	bufferCluster := "buffer-cluster"
	expectedTags := []string{"tag:low"}
	assert.EqualValues(t, []*model.Manifest{
		{
			Type: int32(1),
			Tags: []string{"tag_for:type1"},
		},
		{
			Type: int32(2),
		},
	}, manifestToSend[0].Manifests)
	assert.EqualValues(t, bufferCluster, manifestToSend[0].ClusterName)
	assert.EqualValues(t, expectedTags, manifestToSend[0].Tags)

	assert.EqualValues(t, []*model.Manifest{
		{
			Type: int32(3),
		},
		{
			Type: int32(4),
		},
	}, manifestToSend[1].Manifests)
	assert.EqualValues(t, bufferCluster, manifestToSend[1].ClusterName)
	assert.EqualValues(t, expectedTags, manifestToSend[1].Tags)

	assert.EqualValues(t, []*model.Manifest{
		{
			Type: int32(5),
		},
	}, manifestToSend[2].Manifests)
	assert.EqualValues(t, bufferCluster, manifestToSend[2].ClusterName)
	assert.EqualValues(t, expectedTags, manifestToSend[2].Tags)
}

func TestNewManifestBuffer(t *testing.T) {
	orchCheck := &OrchestratorCheck{
		clusterID: "test-cluster-id",
		orchestratorConfig: &orchcfg.OrchestratorConfig{
			KubeClusterName:          "test-cluster-name",
			MaxPerMessage:            100,
			MaxWeightPerMessageBytes: 1000000,
		},
	}

	mb := NewManifestBuffer(orchCheck)

	// Verify the buffer was properly initialized
	assert.NotNil(t, mb)
	assert.NotNil(t, mb.Cfg)
	assert.NotNil(t, mb.ManifestChan)
	assert.NotNil(t, mb.stopCh)
	assert.NotNil(t, mb.bufferedManifests)
	assert.Equal(t, 0, len(mb.bufferedManifests))
	assert.Equal(t, cap(mb.bufferedManifests), mb.Cfg.MaxBufferedManifests)

	// Verify configuration was copied correctly
	assert.Equal(t, orchCheck.clusterID, mb.Cfg.ClusterID)
	assert.Equal(t, orchCheck.orchestratorConfig.KubeClusterName, mb.Cfg.KubeClusterName)
	assert.Equal(t, orchCheck.orchestratorConfig.MaxPerMessage, mb.Cfg.MaxPerMessage)
	assert.Equal(t, orchCheck.orchestratorConfig.MaxWeightPerMessageBytes, mb.Cfg.MaxWeightPerMessageBytes)
}

func TestFlushManifest(t *testing.T) {
	// Reset global stats
	manifestToSend = []*model.CollectorManifest{}

	mb := getManifestBuffer(t)
	sender := getSender(t)
	testManifests := []interface{}{
		&model.Manifest{
			Type:            int32(1),
			Uid:             "manifest-1",
			ResourceVersion: "v1",
			Content:         []byte(`{"kind":"Pod","metadata":{"name":"test-pod-1"}}`),
			Version:         "v1",
			ContentType:     "json",
			Tags:            []string{"test:tag1"},
		},
		&model.Manifest{
			Type:            int32(2),
			Uid:             "manifest-2",
			ResourceVersion: "v2",
			Content:         []byte(`{"kind":"Service","metadata":{"name":"test-service-1"}}`),
			Version:         "v1",
			ContentType:     "json",
			Tags:            []string{"test:tag2"},
		},
	}

	mb.bufferedManifests = testManifests

	// Verify buffer has manifests before flush
	assert.Len(t, mb.bufferedManifests, 2)

	mb.flushManifest(sender)

	require.Len(t, manifestToSend, 1)
	sentManifest := manifestToSend[0]

	// Verify the manifest contains expected data
	assert.Equal(t, "buffer-cluster", sentManifest.ClusterName)
	assert.Equal(t, mb.Cfg.ClusterID, sentManifest.ClusterId)
	assert.Equal(t, []string{"tag:low"}, sentManifest.Tags) // From ExtraTags in test setup
	assert.Equal(t, int32(1), sentManifest.GroupId)         // MsgGroupRef.Inc() should return 1 for first call
	assert.Equal(t, int32(1), sentManifest.GroupSize)       // Only one chunk

	// Verify manifests are correctly included
	assert.Len(t, sentManifest.Manifests, 2)
	for i := range len(testManifests) {
		assert.Equal(t, testManifests[i].(*model.Manifest).Uid, sentManifest.Manifests[i].Uid)
		assert.Equal(t, testManifests[i].(*model.Manifest).Tags, sentManifest.Manifests[i].Tags)
	}

	// Verify buffer was cleared after flush
	assert.Len(t, mb.bufferedManifests, 0)
	// Capacity should remain the same, only length is reset to 0
	assert.Equal(t, mb.Cfg.MaxBufferedManifests, cap(mb.bufferedManifests))

	// Verify stats were updated
	// Check that manifestFlushed expvar was updated
	assert.Equal(t, int64(2), manifestFlushed.Value())    // 2 manifests were flushed
	assert.Equal(t, int64(1), bufferFlushedTotal.Value()) // Buffer was flushed once
}

// getSender returns a mock Sender
// When calling OrchestratorManifest, it adds the messges to a global var manifestToSend
func getSender(t *testing.T) *mocksender.MockSender {
	sender := mocksender.NewMockSender(checkid.ID(rune(1)))
	sender.On("OrchestratorManifest", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		arg := args.Get(0).([]model.MessageBody)
		require.GreaterOrEqual(t, len(arg), 1)
		a := arg[0].(*model.CollectorManifest)
		manifestToSend = append(manifestToSend, a)
	})
	return sender
}

// getManifestBuffer returns a manifest buffer for test with buffer size = 2
func getManifestBuffer(t *testing.T) *ManifestBuffer {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetGlobalTags([]string{"tag:low"}, []string{"tag:orch"}, []string{"tag:high"}, []string{"tag:std"})

	orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

	// Configure the check properly to get ExtraTags set
	mockSenderManager := mocksender.CreateDefaultDemultiplexer()
	_ = orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")

	// Override the cluster name for the test
	orchCheck.orchestratorConfig.KubeClusterName = "buffer-cluster"

	mb := NewManifestBuffer(orchCheck)
	mb.Cfg.MaxBufferedManifests = 2
	mb.Cfg.ManifestBufferFlushInterval = 3 * time.Second
	mb.Cfg.MsgGroupRef = atomic.NewInt32(0)

	return mb
}
