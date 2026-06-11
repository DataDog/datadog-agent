// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package demultiplexerendpointimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/lookbacksender"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
)

// fakeLookbackDemux embeds the demultiplexer component interface so it satisfies
// the demultiplexerEndpoint.demux field, overriding only the methods the
// handlers exercise. Any other call would panic on the nil embed, which is
// intentional for these focused tests.
type fakeLookbackDemux struct {
	demultiplexerComp.Component
	count         int
	err           error
	senderManager *lookbacksender.SenderManager
}

func (f fakeLookbackDemux) DumpLookback() (int, error) { return f.count, f.err }

func (f fakeLookbackDemux) LookbackSenderManager() *lookbacksender.SenderManager {
	return f.senderManager
}

func TestDumpLookbackEndpointSuccess(t *testing.T) {
	ep := demultiplexerEndpoint{demux: fakeLookbackDemux{count: 4}, log: logmock.New(t)}

	rec := httptest.NewRecorder()
	ep.dumpLookback(rec, httptest.NewRequest(http.MethodPost, "/agent/metric-lookback-dump", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp lookbackDumpResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 4, resp.SeriesDumped)
}

func TestDumpLookbackEndpointError(t *testing.T) {
	ep := demultiplexerEndpoint{
		demux: fakeLookbackDemux{err: errors.New("metric lookback is disabled")},
		log:   logmock.New(t),
	}

	rec := httptest.NewRecorder()
	ep.dumpLookback(rec, httptest.NewRequest(http.MethodPost, "/agent/metric-lookback-dump", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var errMap map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errMap))
	assert.Contains(t, errMap["error"], "disabled")
}

func TestSeedLookbackEndpointSuccess(t *testing.T) {
	buffer := ringbuffer.New(ringbuffer.Options{Capacity: 10, ShardCount: 1})
	manager := lookbacksender.NewSenderManager(context.Background(), "default-host", buffer, func() float64 { return 123 })
	ep := demultiplexerEndpoint{
		demux:  fakeLookbackDemux{senderManager: manager},
		config: configmock.NewMockWithOverrides(t, map[string]interface{}{"metric_lookback.debug_seed.enabled": true}),
		log:    logmock.New(t),
	}

	payload := []byte(`{"check_id":"demo-shadow","metric":"demo.lookback.shadow","value":42,"tags":["demo:lookback"],"type":"gauge"}`)
	rec := httptest.NewRecorder()
	ep.seedLookback(rec, httptest.NewRequest(http.MethodPost, "/agent/metric-lookback-seed", bytes.NewReader(payload)))

	require.Equal(t, http.StatusOK, rec.Code)
	var resp lookbackSeedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "demo-shadow", resp.CheckID)
	assert.Equal(t, "demo.lookback.shadow", resp.Metric)
	assert.Equal(t, "gauge", resp.Type)
	assert.Equal(t, 1, resp.SamplesBuffered)

	stats := buffer.Stats()
	assert.Equal(t, 1, stats.Records)
	assert.Equal(t, 1, stats.ActiveContexts)
	assert.Equal(t, uint64(1), stats.AppendedSamples)
}

func TestSeedLookbackEndpointDisabled(t *testing.T) {
	ep := demultiplexerEndpoint{
		demux:  fakeLookbackDemux{},
		config: configmock.NewMockWithOverrides(t, map[string]interface{}{"metric_lookback.debug_seed.enabled": false}),
		log:    logmock.New(t),
	}

	rec := httptest.NewRecorder()
	ep.seedLookback(rec, httptest.NewRequest(http.MethodPost, "/agent/metric-lookback-seed", bytes.NewReader([]byte(`{"metric":"demo"}`))))

	require.Equal(t, http.StatusForbidden, rec.Code)
	var errMap map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errMap))
	assert.Contains(t, errMap["error"], "disabled")
}
