// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStatsdDirect(t *testing.T) {
	opts := DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = time.Hour
	opts.DontStartForwarders = true
	demuxDeps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(demuxDeps.Log, NewForwarderTest(demuxDeps.Log), demuxDeps.OrchestratorFwd, opts, demuxDeps.EventPlatform, demuxDeps.HaAgent, demuxDeps.Compressor, demuxDeps.Tagger, "")

	hostnameComp := fxutil.Test[hostnameinterface.Mock](t,
		fx.Options(
			hostnameinterface.MockModule(),
			fx.Replace(hostnameinterface.MockHostname("my-hostname")),
		),
	)

	statsd, err := NewStatsdDirect(demux, hostnameComp)
	require.NoError(t, err)

	err = statsd.Gauge("test.gauge", 1.0, []string{"tag1:value1", "tag2:value2"}, 1.0)
	require.NoError(t, err)

	samples := <-demux.statsd.workers[0].samplesChan
	require.Len(t, samples, 1)

	sample := samples[0]
	require.Equal(t, "test.gauge", sample.Name)
	require.Equal(t, 1.0, sample.Value)
	require.Equal(t, []string{"tag1:value1", "tag2:value2"}, sample.Tags)
	require.Equal(t, "my-hostname", sample.Host)
	require.Equal(t, 1.0, sample.SampleRate)
	require.Equal(t, metrics.GaugeType, sample.Mtype)
}
