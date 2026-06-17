// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	"github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type recordingLookbackTrigger struct {
	mu           sync.Mutex
	observations []metrics.MetricSample
}

func (r *recordingLookbackTrigger) Observe(name string, value float64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = append(r.observations, metrics.MetricSample{Name: name, Value: value})
	return true
}

func (r *recordingLookbackTrigger) snapshot() []metrics.MetricSample {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]metrics.MetricSample(nil), r.observations...)
}

func initLookbackTriggerTestDemux(t *testing.T, trigger LookbackTrigger) *AgentDemultiplexer {
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
	options.LookbackTriggerFactory = func(LookbackDumper) LookbackTrigger {
		return trigger
	}
	return InitAndStartAgentDemultiplexerForTest(deps, options, "lookback-trigger-host")
}

func TestLookbackTriggerObservesDogStatsDSamples(t *testing.T) {
	configmock.New(t)
	trigger := &recordingLookbackTrigger{}
	demux := initLookbackTriggerTestDemux(t, trigger)
	defer demux.Stop()

	demux.AggregateSample(metrics.MetricSample{Name: "single.signal", Value: 1})
	demux.AggregateSamples(TimeSamplerID(0), metrics.MetricSampleBatch{
		{Name: "batch.signal.a", Value: 2},
		{Name: "batch.signal.b", Value: 3},
	})

	assert.Equal(t, []metrics.MetricSample{
		{Name: "single.signal", Value: 1},
		{Name: "batch.signal.a", Value: 2},
		{Name: "batch.signal.b", Value: 3},
	}, trigger.snapshot())
}

func TestLookbackTriggerDisabledIsNoop(t *testing.T) {
	configmock.New(t)
	demux := initLookbackTriggerTestDemux(t, nil)
	defer demux.Stop()

	assert.NotPanics(t, func() {
		demux.AggregateSample(metrics.MetricSample{Name: "single.signal", Value: 1})
		demux.AggregateSamples(TimeSamplerID(0), metrics.MetricSampleBatch{{Name: "batch.signal", Value: 2}})
	})
}

var _ LookbackTrigger = (*recordingLookbackTrigger)(nil)
