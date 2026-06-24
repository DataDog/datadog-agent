// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	"github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	aggregatorsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type recordingLookbackRetention struct {
	manager aggregatorsender.SenderManager

	newSenderManagerCalls int
	dumpCalls             int
	dumpRangeCalls        int
	dumpCount             int
	dumpErr               error
	lastSerializer        serializer.MetricSerializer
	lastFrom              time.Time
	lastTo                time.Time
}

func (r *recordingLookbackRetention) NewSenderManager(context.Context) aggregatorsender.SenderManager {
	r.newSenderManagerCalls++
	return r.manager
}

func (r *recordingLookbackRetention) Dump(metricSerializer serializer.MetricSerializer) (int, error) {
	r.dumpCalls++
	r.lastSerializer = metricSerializer
	return r.dumpCount, r.dumpErr
}

func (r *recordingLookbackRetention) DumpRange(metricSerializer serializer.MetricSerializer, from, to time.Time) (int, error) {
	r.dumpRangeCalls++
	r.lastSerializer = metricSerializer
	r.lastFrom = from
	r.lastTo = to
	return r.dumpCount, r.dumpErr
}

type recordingSenderManager struct{}

func (recordingSenderManager) GetSender(checkid.ID) (aggregatorsender.Sender, error) { return nil, nil }
func (recordingSenderManager) SetSender(aggregatorsender.Sender, checkid.ID) error   { return nil }
func (recordingSenderManager) DestroySender(checkid.ID)                              {}
func (recordingSenderManager) GetDefaultSender() (aggregatorsender.Sender, error)    { return nil, nil }

// initLookbackTestDemux builds a real demultiplexer through the normal
// construction path so the metric_lookback wiring under test is exercised.
func initLookbackTestDemux(t *testing.T, retention LookbackRetention) *AgentDemultiplexer {
	t.Helper()
	deps := fxutil.Test[TestDeps](t,
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		defaultforwardermock.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		mock.Module(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistmock.MockModule(),
	)
	options := demuxTestOptions()
	options.LookbackRetention = retention
	return InitAndStartAgentDemultiplexerForTest(deps, options, "lookback-host")
}

func TestLookbackSenderManagerUsesConfiguredRetention(t *testing.T) {
	configmock.New(t)
	manager := recordingSenderManager{}
	retention := &recordingLookbackRetention{manager: manager}
	demux := initLookbackTestDemux(t, retention)
	defer demux.Stop()

	assert.Equal(t, manager, demux.LookbackSenderManager(context.Background()))
	assert.Equal(t, 1, retention.newSenderManagerCalls)
}

// TestLookbackRetentionNotUsedByNormalFlow is the key guarantee: metrics sent
// through the normal check sender path must NOT interact with metric lookback
// retention. Only the explicit shadow sender path asks retention for senders.
func TestLookbackRetentionNotUsedByNormalFlow(t *testing.T) {
	configmock.New(t)
	retention := &recordingLookbackRetention{manager: recordingSenderManager{}}
	demux := initLookbackTestDemux(t, retention)
	defer demux.Stop()

	// Normal check sender path.
	normal, err := demux.GetSender(checkid.ID("normal:check"))
	require.NoError(t, err)
	defer demux.DestroySender(checkid.ID("normal:check"))
	normal.Gauge("normal.gauge", 1, "", []string{"x:1"})
	normal.Commit()

	// Wait until the aggregator has drained the normal sender's items.
	require.Eventually(t, demux.aggregator.IsInputQueueEmpty, 2*time.Second, 5*time.Millisecond)
	assert.Zero(t, retention.newSenderManagerCalls, "normal metric flow must not request a lookback sender manager")

	assert.NotNil(t, demux.LookbackSenderManager(context.Background()))
	assert.Equal(t, 1, retention.newSenderManagerCalls)
}

func TestLookbackDisabled(t *testing.T) {
	configmock.New(t)
	demux := initLookbackTestDemux(t, nil)
	defer demux.Stop()

	assert.Nil(t, demux.LookbackSenderManager(context.Background()))
}

func TestDumpLookbackDelegatesToConfiguredRetention(t *testing.T) {
	configmock.New(t)
	retention := &recordingLookbackRetention{dumpCount: 2}
	demux := &AgentDemultiplexer{
		log:               logmock.New(t),
		lookbackRetention: retention,
		dataOutputs:       dataOutputs{sharedSerializer: &MockSerializerIterableSerie{}},
	}

	count, err := demux.DumpLookback()
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, 1, retention.dumpRangeCalls)
	assert.NotNil(t, retention.lastSerializer)
	assert.True(t, retention.lastFrom.IsZero())
	assert.True(t, retention.lastTo.IsZero())
}

func TestDumpLookbackRangeDelegatesToConfiguredRetention(t *testing.T) {
	configmock.New(t)
	retention := &recordingLookbackRetention{dumpCount: 2}
	demux := &AgentDemultiplexer{
		log:               logmock.New(t),
		lookbackRetention: retention,
		dataOutputs:       dataOutputs{sharedSerializer: &MockSerializerIterableSerie{}},
	}
	from := time.Unix(10, 0)
	to := time.Unix(20, 0)

	count, err := demux.DumpLookbackRange(from, to)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, 1, retention.dumpRangeCalls)
	assert.NotNil(t, retention.lastSerializer)
	assert.Equal(t, from, retention.lastFrom)
	assert.Equal(t, to, retention.lastTo)
}

func TestDumpLookbackDisabledReturnsError(t *testing.T) {
	configmock.New(t)
	demux := &AgentDemultiplexer{
		log:         logmock.New(t),
		dataOutputs: dataOutputs{sharedSerializer: &MockSerializerIterableSerie{}},
	}
	_, err := demux.DumpLookback()
	require.Error(t, err)
}

func TestDumpLookbackSerializerUnavailableReturnsError(t *testing.T) {
	configmock.New(t)
	demux := &AgentDemultiplexer{
		log:               logmock.New(t),
		lookbackRetention: &recordingLookbackRetention{},
	}
	_, err := demux.DumpLookback()
	require.Error(t, err)
}

var _ LookbackRetention = (*recordingLookbackRetention)(nil)
var _ aggregatorsender.SenderManager = recordingSenderManager{}
var _ serializer.MetricSerializer = (*MockSerializerIterableSerie)(nil)
