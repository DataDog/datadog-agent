// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func demuxTestOptions() AgentDemultiplexerOptions {
	opts := DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = time.Hour
	opts.DontStartForwarders = true
	return opts
}

func TestDemuxIsSetAsGlobalInstance(t *testing.T) {
	require := require.New(t)

	opts := demuxTestOptions()
	deps := createDemuxDeps(t, opts, eventplatformimpl.NewDefaultParams())
	demux := deps.Demultiplexer

	require.NotNil(demux)
	require.NotNil(demux.aggregator)
	demux.Stop(false)
}

func TestDemuxForwardersCreated(t *testing.T) {
	require := require.New(t)

	// default options should have created all forwarders except for the orchestrator
	// forwarders since we're not in a cluster-agent environment

	opts := demuxTestOptions()

	deps := createDemuxDeps(t, opts, eventplatformimpl.NewDefaultParams())
	demux := deps.Demultiplexer

	require.NotNil(demux)
	_, found := deps.EventPlatformFwd.Get()
	require.True(found)
	_, found = deps.OrchestratorFwd.Get()
	require.Equal(orchestratorForwarderSupport, found)
	require.NotNil(deps.SharedForwarder)
	demux.Stop(false)

	// options no event platform forwarder

	opts = demuxTestOptions()
	deps = createDemuxDeps(t, opts, eventplatformimpl.Params{UseEventPlatformForwarder: false})
	demux = deps.Demultiplexer
	require.NotNil(demux)
	_, found = deps.EventPlatformFwd.Get()
	require.False(found)
	_, found = deps.OrchestratorFwd.Get()
	require.Equal(orchestratorForwarderSupport, found)
	require.NotNil(deps.SharedForwarder)
	demux.Stop(false)

	// options noop event platform forwarder

	opts = demuxTestOptions()
	deps = createDemuxDeps(t, opts, eventplatformimpl.Params{UseNoopEventPlatformForwarder: true})
	demux = deps.Demultiplexer
	require.NotNil(demux)
	_, found = deps.EventPlatformFwd.Get()
	require.True(found)
	_, found = deps.OrchestratorFwd.Get()
	require.Equal(orchestratorForwarderSupport, found)
	require.NotNil(deps.SharedForwarder)
	demux.Stop(false)

	// now, simulate a cluster-agent environment and enabled the orchestrator feature

	oee := pkgconfigsetup.Datadog().Get("orchestrator_explorer.enabled")
	cre := pkgconfigsetup.Datadog().Get("clc_runner_enabled")
	ecp := pkgconfigsetup.Datadog().Get("extra_config_providers")
	defer func() {
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.enabled", oee)
		pkgconfigsetup.Datadog().SetWithoutSource("clc_runner_enabled", cre)
		pkgconfigsetup.Datadog().SetWithoutSource("extra_config_providers", ecp)
	}()
	pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.enabled", true)
	pkgconfigsetup.Datadog().SetWithoutSource("clc_runner_enabled", true)
	pkgconfigsetup.Datadog().SetWithoutSource("extra_config_providers", []string{"clusterchecks"})

	// since we're running the tests with -tags orchestrator and we've enabled the
	// needed feature above, we should have an orchestrator forwarder instantiated now

	opts = demuxTestOptions()
	deps = createDemuxDeps(t, opts, eventplatformimpl.NewDefaultParams())
	demux = deps.Demultiplexer
	require.NotNil(demux)
	_, found = deps.EventPlatformFwd.Get()
	require.True(found)
	require.NotNil(deps.SharedForwarder)
	demux.Stop(false)

	// options no orchestrator forwarder

	opts = demuxTestOptions()
	params := orchestratorForwarderImpl.NewDisabledParams()
	deps = createDemuxDepsWithOrchestratorFwd(t, opts, params, eventplatformimpl.NewDefaultParams())
	demux = deps.Demultiplexer
	require.NotNil(demux)
	_, found = deps.EventPlatformFwd.Get()
	require.True(found)
	_, found = deps.OrchestratorFwd.Get()
	require.False(found)
	require.NotNil(deps.SharedForwarder)
	demux.Stop(false)

	// options noop orchestrator forwarder

	opts = demuxTestOptions()
	params = orchestratorForwarderImpl.NewNoopParams()
	deps = createDemuxDepsWithOrchestratorFwd(t, opts, params, eventplatformimpl.NewDefaultParams())
	demux = deps.Demultiplexer
	require.NotNil(demux)
	_, found = deps.EventPlatformFwd.Get()
	require.True(found)
	_, found = deps.OrchestratorFwd.Get()
	require.True(found)
	require.NotNil(deps.SharedForwarder)
	demux.Stop(false)
}

func TestDemuxSerializerCreated(t *testing.T) {
	require := require.New(t)

	// default options should have created all forwarders

	opts := demuxTestOptions()
	deps := createDemuxDeps(t, opts, eventplatformimpl.NewDefaultParams())
	demux := deps.Demultiplexer

	require.NotNil(demux)
	require.NotNil(demux.sharedSerializer)
	demux.Stop(false)
}

func TestDemuxFlushAggregatorToSerializer(t *testing.T) {
	require := require.New(t)
	var defaultCheckID checkid.ID // empty checkid.ID is the default sender ID

	// default options should have created all forwarders

	opts := demuxTestOptions()
	opts.FlushInterval = time.Hour
	deps := createDemuxDeps(t, opts, eventplatformimpl.NewDefaultParams())
	demux := initAgentDemultiplexer(deps.Log, deps.SharedForwarder, deps.OrchestratorFwd, opts, deps.EventPlatformFwd, deps.Compressor, nooptagger.NewTaggerClient(), "")
	demux.Aggregator().tlmContainerTagsEnabled = false
	require.NotNil(demux)
	require.NotNil(demux.aggregator)
	require.NotNil(demux.sharedSerializer)

	sender, err := demux.GetDefaultSender()
	require.NoError(err)
	sender.Count("my.check.metric", 1.0, "", []string{"team:agent-core", "dev:remeh"})
	sender.Count("my.second.check.metric", 5.0, "", []string{"team:agent-core", "dev:remeh"})
	sender.Count("my.third.check.metric", 42.0, "", []string{"team:agent-core", "dev:remeh"})
	sender.Commit()

	// we have to run the aggregator for it to process these samples, but we
	// want to stop it to read its samplers information, which is never happening
	// in real life but which can cause races during this unit test.
	// we want to make sure the aggregator has time to process these samples
	// in its select before shutting it down, unfortunately, there is no other
	// way today than giving it some time to run
	go func() {
		time.Sleep(250 * time.Millisecond)
		demux.aggregator.stopChan <- struct{}{}
	}()
	demux.aggregator.run()

	series, sketches := demux.aggregator.checkSamplers[defaultCheckID].flush()
	require.Len(series, 3)
	require.Len(sketches, 0)
}

func TestGetDogStatsDWorkerAndPipelineCount(t *testing.T) {
	pc := pkgconfigsetup.Datadog().GetInt("dogstatsd_pipeline_count")
	aa := pkgconfigsetup.Datadog().GetInt("dogstatsd_pipeline_autoadjust")
	defer func() {
		pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_count", pc)
		pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_autoadjust", aa)
	}()

	assert := assert.New(t)

	// auto-adjust

	pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_autoadjust", true)

	dsdWorkers, pipelines := getDogStatsDWorkerAndPipelineCount(16)
	assert.Equal(8, dsdWorkers)
	assert.Equal(7, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(11)
	assert.Equal(5, dsdWorkers)
	assert.Equal(4, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(8)
	assert.Equal(4, dsdWorkers)
	assert.Equal(3, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(4)
	assert.Equal(2, dsdWorkers)
	assert.Equal(1, pipelines)

	// no auto-adjust

	pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_autoadjust", false)
	pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_count", pc) // default value

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(16)
	assert.Equal(14, dsdWorkers)
	assert.Equal(1, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(11)
	assert.Equal(9, dsdWorkers)
	assert.Equal(1, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(8)
	assert.Equal(6, dsdWorkers)
	assert.Equal(1, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(4)
	assert.Equal(2, dsdWorkers)
	assert.Equal(1, pipelines)

	// no auto-adjust + pipeline count

	pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_autoadjust", false)
	pkgconfigsetup.Datadog().SetWithoutSource("dogstatsd_pipeline_count", 4)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(16)
	assert.Equal(11, dsdWorkers)
	assert.Equal(4, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(11)
	assert.Equal(6, dsdWorkers)
	assert.Equal(4, pipelines)

	dsdWorkers, pipelines = getDogStatsDWorkerAndPipelineCount(4)
	assert.Equal(2, dsdWorkers)
	assert.Equal(4, pipelines)
}

func createDemuxDeps(t *testing.T, opts AgentDemultiplexerOptions, eventPlatformParams eventplatformimpl.Params) aggregatorDeps {
	return createDemuxDepsWithOrchestratorFwd(t, opts, orchestratorForwarderImpl.NewDefaultParams(), eventPlatformParams)
}

type internalDemutiplexerDeps struct {
	TestDeps
	OrchestratorForwarder orchestratorForwarder.Component
	Eventplatform         eventplatform.Component
	Compressor            compression.Component
}

func createDemuxDepsWithOrchestratorFwd(
	t *testing.T,
	opts AgentDemultiplexerOptions,
	orchestratorParams orchestratorForwarderImpl.Params,
	eventPlatformParams eventplatformimpl.Params) aggregatorDeps {
	modules := fx.Options(
		defaultforwarder.MockModule(),
		core.MockBundle(),
		orchestratorForwarderImpl.Module(orchestratorParams),
		eventplatformimpl.Module(eventPlatformParams),
		eventplatformreceiverimpl.Module(),
		compressionimpl.MockModule(),
	)
	deps := fxutil.Test[internalDemutiplexerDeps](t, modules)

	return aggregatorDeps{
		TestDeps:         deps.TestDeps,
		Demultiplexer:    InitAndStartAgentDemultiplexer(deps.Log, deps.SharedForwarder, deps.OrchestratorForwarder, opts, deps.Eventplatform, deps.Compressor, nooptagger.NewTaggerClient(), ""),
		OrchestratorFwd:  deps.OrchestratorForwarder,
		EventPlatformFwd: deps.Eventplatform,
	}
}
