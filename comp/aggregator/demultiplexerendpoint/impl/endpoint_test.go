// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerendpointimpl

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type fakeContextDumper []aggregator.ContextDebugRepr

func (d fakeContextDumper) DumpDogstatsdContexts(w io.Writer) error {
	enc := json.NewEncoder(w)
	for _, context := range d {
		if err := enc.Encode(context); err != nil {
			return err
		}
	}
	return nil
}

func TestValidateTopRequest(t *testing.T) {
	for _, request := range []topRequest{
		{Source: topSourceLive, NumMetrics: 0, NumTags: 5},
		{Source: topSourceLive, NumMetrics: maxNumMetrics + 1, NumTags: 5},
		{Source: topSourceLive, NumMetrics: 10, NumTags: 0},
		{Source: topSourceLive, NumMetrics: 10, NumTags: maxNumTags + 1},
		{Source: "saved", NumMetrics: 10, NumTags: 5},
	} {
		require.Error(t, validateTopRequest(request))
	}

	require.NoError(t, validateTopRequest(topRequest{Source: topSourceLive, NumMetrics: 10, NumTags: 5}))
	require.NoError(t, validateTopRequest(topRequest{Source: topSourceDump, NumMetrics: 10, NumTags: 5}))
}

func TestTopDogstatsdContextsFromDump(t *testing.T) {
	endpoint := demultiplexerEndpoint{
		demux: fakeContextDumper{
			{Name: "requests", MetricTags: []string{"env:prod"}},
		},
		runPath: t.TempDir(),
	}
	_, err := endpoint.writeDogstatsdContexts()
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-top", bytes.NewBufferString(`{"source":"dump"}`))
	endpoint.topDogstatsdContexts(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"source": "dump",
		"metrics": [{
			"name": "requests",
			"contexts": 1,
			"tags": [{"key": "env", "unique_values": 1}]
		}]
	}`, recorder.Body.String())
}

func TestTopDogstatsdContextsDefaultsToLive(t *testing.T) {
	endpoint := demultiplexerEndpoint{
		demux: fakeContextDumper{
			{Name: "requests", MetricTags: []string{"env:prod"}},
		},
		runPath: t.TempDir(),
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-top", bytes.NewBufferString(`{}`))
	endpoint.topDogstatsdContexts(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"source": "live",
		"metrics": [{
			"name": "requests",
			"contexts": 1,
			"tags": [{"key": "env", "unique_values": 1}]
		}]
	}`, recorder.Body.String())
}

func TestTopDogstatsdContextsStrictlyLimitsSingleRemainders(t *testing.T) {
	endpoint := demultiplexerEndpoint{
		demux: fakeContextDumper{
			{Name: "first", MetricTags: []string{"alpha:1", "beta:1", "gamma:1"}},
			{Name: "first", MetricTags: []string{"alpha:2", "beta:1", "gamma:1"}},
			{Name: "second"},
			{Name: "third"},
		},
		runPath: t.TempDir(),
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-top", bytes.NewBufferString(`{"num_metrics":2,"num_tags":2,"source":"live"}`))
	endpoint.topDogstatsdContexts(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"source": "live",
		"metrics": [
			{
				"name": "first",
				"contexts": 2,
				"tags": [
					{"key": "alpha", "unique_values": 2},
					{"key": "beta", "unique_values": 1}
				],
				"other_tags": 1,
				"other_tag_values": 1
			},
			{"name": "second", "contexts": 1, "tags": []}
		],
		"other_metrics": 1,
		"other_contexts": 1
	}`, recorder.Body.String())
}

func TestTopDogstatsdContextsRejectsLiveWhenDataPlaneOwnsDogstatsd(t *testing.T) {
	endpoint := demultiplexerEndpoint{
		demux:                fakeContextDumper{{Name: "requests"}},
		runPath:              t.TempDir(),
		dogstatsdOnDataPlane: true,
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-top", bytes.NewBufferString(`{}`))
	endpoint.topDogstatsdContexts(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestDumpDogstatsdContextsRejectsWhenDataPlaneOwnsDogstatsd(t *testing.T) {
	endpoint := demultiplexerEndpoint{dogstatsdOnDataPlane: true}
	recorder := httptest.NewRecorder()

	endpoint.dumpDogstatsdContexts(recorder, httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-dump", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Agent Data Plane")
}
