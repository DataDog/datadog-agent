// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package demultiplexerendpointimpl

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// fakeLookbackDemux embeds the demultiplexer component interface so it satisfies
// the demultiplexerEndpoint.demux field, overriding only DumpLookback (the only
// method the handler exercises). Any other call would panic on the nil embed,
// which is intentional for this focused test.
type fakeLookbackDemux struct {
	demultiplexerComp.Component
	count int
	err   error
}

func (f fakeLookbackDemux) DumpLookback() (int, error) { return f.count, f.err }

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
