// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// initLookbackTestDemux builds a real demultiplexer through the normal
// construction path so the metric_lookback wiring under test is exercised.
func initLookbackTestDemux(t *testing.T) *AgentDemultiplexer {
	t.Helper()
	deps := fxutil.Test[TestDeps](t,
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		defaultforwardermock.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		haagentmock.Module(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistmock.MockModule(),
	)
	return InitAndStartAgentDemultiplexerForTest(deps, demuxTestOptions(), "lookback-host")
}

// TestLookbackSenderManagerWiredByDefault verifies that, with the default
// configuration, the demultiplexer exposes a lookback shadow sender manager and
// that samples committed through one of its senders land in the ring buffer.
func TestLookbackSenderManagerWiredByDefault(t *testing.T) {
	configmock.New(t) // metric_lookback.enabled defaults to true
	demux := initLookbackTestDemux(t)
	defer demux.Stop()

	require.NotNil(t, demux.LookbackSenderManager(), "lookback sender manager should be wired by default")
	require.NotNil(t, demux.lookbackBuffer)

	sender, err := demux.LookbackSenderManager().GetSender(checkid.ID("shadow:check"))
	require.NoError(t, err)
	sender.Gauge("shadow.gauge", 3, "", []string{"a:1"})
	sender.Commit() // lookback sender writes synchronously to the buffer

	assert.Equal(t, 1, demux.lookbackBuffer.Stats().Records)
}

// TestLookbackBufferNotFedByNormalFlow is the key guarantee: metrics sent
// through the normal check sender path must NOT be retained in the lookback
// buffer. Only the lookback shadow sender feeds it.
func TestLookbackBufferNotFedByNormalFlow(t *testing.T) {
	configmock.New(t)
	demux := initLookbackTestDemux(t)
	defer demux.Stop()
	require.NotNil(t, demux.lookbackBuffer)

	// Normal check sender path.
	normal, err := demux.GetSender(checkid.ID("normal:check"))
	require.NoError(t, err)
	defer demux.DestroySender(checkid.ID("normal:check"))
	normal.Gauge("normal.gauge", 1, "", []string{"x:1"})
	normal.Commit()

	// Wait until the aggregator has drained the normal sender's items.
	require.Eventually(t, demux.aggregator.IsInputQueueEmpty, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, demux.lookbackBuffer.Stats().Records, "normal metric flow must not feed the lookback buffer")

	// The shadow sender path, by contrast, does feed it.
	shadow, err := demux.LookbackSenderManager().GetSender(checkid.ID("normal:check"))
	require.NoError(t, err)
	shadow.Gauge("shadow.gauge", 2, "", nil)
	shadow.Commit()
	assert.Equal(t, 1, demux.lookbackBuffer.Stats().Records)
}

// TestLookbackDisabled verifies the manager and buffer are absent when disabled.
func TestLookbackDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("metric_lookback.enabled", false)
	demux := initLookbackTestDemux(t)
	defer demux.Stop()

	assert.Nil(t, demux.LookbackSenderManager())
	assert.Nil(t, demux.lookbackBuffer)
}
