// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	providerMocks "github.com/DataDog/datadog-agent/pkg/util/containers/providers/mock"

	"github.com/stretchr/testify/require"
)

func init() {
	providers.Register(providerMocks.FakeContainerImpl{})
}

func resetDemuxInstance(require *require.Assertions) {
	if demultiplexerInstance != nil {
		demultiplexerInstance.Stop(false)
		require.Nil(demultiplexerInstance)
	}
}

func demuxTestOptions() DemultiplexerOptions {
	opts := DefaultDemultiplexerOptions(nil)
	opts.FlushInterval = time.Hour
	opts.DontStartForwarders = true
	return opts
}

func TestDemuxIsSetAsGlobalInstance(t *testing.T) {
	require := require.New(t)

	resetDemuxInstance(require)

	opts := demuxTestOptions()
	demux := InitAndStartAgentDemultiplexer(opts, "")

	require.NotNil(demux)
	require.NotNil(demux.aggregator)
	require.Equal(demux, demultiplexerInstance)

	demux.Stop(false)
}

func TestDemuxForwardersCreated(t *testing.T) {
	require := require.New(t)

	resetDemuxInstance(require)

	// default options should have created all forwarders except for the orchestrator
	// forwarders since we're not in a cluster-agent environment

	opts := demuxTestOptions()
	demux := InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.forwarders.eventPlatform)
	require.Nil(demux.forwarders.orchestrator)
	require.NotNil(demux.forwarders.shared)
	demux.Stop(false)

	// options no event platform forwarder

	opts = demuxTestOptions()
	opts.UseEventPlatformForwarder = false
	demux = InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.Nil(demux.forwarders.eventPlatform)
	require.Nil(demux.forwarders.orchestrator)
	require.NotNil(demux.forwarders.shared)
	demux.Stop(false)

	// options noop event platform forwarder

	opts = demuxTestOptions()
	opts.UseNoopEventPlatformForwarder = true
	demux = InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.forwarders.eventPlatform)
	require.Nil(demux.forwarders.orchestrator)
	require.NotNil(demux.forwarders.shared)
	demux.Stop(false)

	// now, simulate a cluster-agent environment and enabled the orchestrator feature

	config.Datadog.Set("orchestrator_explorer.enabled", true)
	config.Datadog.Set("clc_runner_enabled", true)
	config.Datadog.Set("extra_config_providers", []string{"clusterchecks"})

	// since we're running the tests with -tags orchestrator and we've enabled the
	// needed feature above, we should have an orchestrator forwarder instantiated now

	opts = demuxTestOptions()
	demux = InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.forwarders.eventPlatform)
	require.NotNil(demux.forwarders.orchestrator)
	require.NotNil(demux.forwarders.shared)
	demux.Stop(false)

	// options no orchestrator forwarder

	opts = demuxTestOptions()
	opts.UseOrchestratorForwarder = false
	demux = InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.forwarders.eventPlatform)
	require.Nil(demux.forwarders.orchestrator)
	require.NotNil(demux.forwarders.shared)
	demux.Stop(false)

	// no options to disable it, but the feature is not enabled

	config.Datadog.Set("orchestrator_explorer.enabled", false)

	opts = demuxTestOptions()
	demux = InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.forwarders.eventPlatform)
	require.Nil(demux.forwarders.orchestrator)
	require.NotNil(demux.forwarders.shared)
	demux.Stop(false)
}

func TestDemuxSerializerCreated(t *testing.T) {
	require := require.New(t)

	resetDemuxInstance(require)

	// default options should have created all forwarders except for the orchestrator
	// forwarders since we're not in a cluster-agent environment

	opts := demuxTestOptions()
	demux := InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.sharedSerializer)
	demux.Stop(false)
}

func TestDemuxFlushAggregatorToSerializer(t *testing.T) {
	require := require.New(t)

	resetDemuxInstance(require)

	// default options should have created all forwarders except for the orchestrator
	// forwarders since we're not in a cluster-agent environment

	opts := demuxTestOptions()
	opts.FlushInterval = time.Hour
	demux := InitAndStartAgentDemultiplexer(opts, "")
	demux.Aggregator().tlmContainerTagsEnabled = false
	require.NotNil(demux)
	require.NotNil(demux.aggregator)
	require.NotNil(demux.sharedSerializer)

	sender, err := GetDefaultSender()
	require.NoError(err)
	sender.Count("my.check.metric", 1.0, "", []string{"team:agent-core", "dev:remeh"})
	sender.Count("my.second.check.metric", 5.0, "", []string{"team:agent-core", "dev:remeh"})
	sender.Count("my.third.check.metric", 42.0, "", []string{"team:agent-core", "dev:remeh"})
	sender.Commit()

	var defaultCheckID check.ID // empty check.ID is the default sender ID

	// nothing should have been flushed to the serializer/samplers yet
	require.Len(demux.aggregator.checkSamplers[defaultCheckID].series, 0)

	// need time for aggregator to run
	time.Sleep(10 * time.Second)

	// flush the data down the pipeline
	demux.FlushAggregatedData(time.Now(), true)
	require.Len(demux.aggregator.checkSamplers[defaultCheckID].series, 3)

	demux.Stop(false)
}
