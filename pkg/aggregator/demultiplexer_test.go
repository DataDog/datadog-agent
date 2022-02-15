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

// Check whether we are built with the +orchestrator build tag
func orchestratorEnabled() bool {
	return buildOrchestratorForwarder() != nil
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

	oee := config.Datadog.Get("orchestrator_explorer.enabled")
	cre := config.Datadog.Get("clc_runner_enabled")
	ecp := config.Datadog.Get("extra_config_providers")
	defer func() {
		config.Datadog.Set("orchestrator_explorer.enabled", oee)
		config.Datadog.Set("clc_runner_enabled", cre)
		config.Datadog.Set("extra_config_providers", ecp)
	}()
	config.Datadog.Set("orchestrator_explorer.enabled", true)
	config.Datadog.Set("clc_runner_enabled", true)
	config.Datadog.Set("extra_config_providers", []string{"clusterchecks"})

	// since we're running the tests with -tags orchestrator and we've enabled the
	// needed feature above, we should have an orchestrator forwarder instantiated now

	opts = demuxTestOptions()
	demux = InitAndStartAgentDemultiplexer(opts, "")
	require.NotNil(demux)
	require.NotNil(demux.forwarders.eventPlatform)
	if orchestratorEnabled() {
		require.NotNil(demux.forwarders.orchestrator)
	} else {
		require.Nil(demux.forwarders.orchestrator)
	}
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
	var defaultCheckID check.ID // empty check.ID is the default sender ID

	// default options should have created all forwarders except for the orchestrator
	// forwarders since we're not in a cluster-agent environment

	opts := demuxTestOptions()
	opts.FlushInterval = time.Hour
	demux := initAgentDemultiplexer(opts, "")
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
