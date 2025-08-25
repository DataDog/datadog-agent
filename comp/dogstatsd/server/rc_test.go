// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd server
package server

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// to validate the outcome of running the config update,
// the callback will update this object
type updateRes map[state.ApplyState][]string

type resultTester struct {
	Acked    int
	Unacked  int
	Errors   int
	Unknowns int
}

// Validating that the Agent isn't crashing on malformed updates.
func TestMalformedFilterListUpdate(t *testing.T) {
	require := require.New(t)
	test := func(results updateRes, tester resultTester) {
		require.Len(results[state.ApplyStateAcknowledged], tester.Acked, "wrong amount of acked")
		require.Len(results[state.ApplyStateUnacknowledged], tester.Unacked, "wrong amount of unacked")
		require.Len(results[state.ApplyStateError], tester.Errors, "wrong amount of errors")
		require.Len(results[state.ApplyStateUnknown], tester.Unknowns, "wrong amount of unknowns")
	}
	reset := func() updateRes { return updateRes{} }

	cfg := make(map[string]interface{})
	cfg["dogstatsd_port"] = listeners.RandomPortName

	deps := fulfillDepsWithConfigOverride(t, cfg)
	s := deps.Server.(*server)

	results := reset()

	callback := func(path string, status state.ApplyStatus) {
		results[status.State] = append(results[status.State], path)
	}

	// this call should fail because the content is malformed
	updates := map[string]state.RawConfig{
		"first": {Config: []byte(`malformedjson":{}}`)},
	}
	s.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()

	// both of these should fail
	updates = map[string]state.RawConfig{
		"first":  {Config: []byte(`malformedjson":{}}`)},
		"second": {Config: []byte(`malformedjson":{}}`)},
	}
	s.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   2,
		Unknowns: 0,
	})
	results = reset()

	// one is incorrect json, the other is an unknown struct we don't know
	// how to interpret, but that'll still be processed without errors (acked)
	updates = map[string]state.RawConfig{
		"first":  {Config: []byte(`malformedjson":{}}`)},
		"second": {Config: []byte(`{"random":"json","field":[]}`)},
	}
	s.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    1,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()

	// two correct ones, with one metric
	updates = map[string]state.RawConfig{
		"first":  {Config: []byte(`{"blocking_metrics":{"by_name":["hello","world"]}`)},
		"second": {Config: []byte(`{"random":"json","field":[]}`)},
	}
	s.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    1,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()

	// nothing
	updates = map[string]state.RawConfig{}
	s.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   0,
		Unknowns: 0,
	})
	results = reset()

	// one config but empty, it should be unparseable
	updates = map[string]state.RawConfig{
		"first": {Config: []byte("")},
	}
	s.onFilterListUpdateCallback(updates, callback)
	test(results, resultTester{
		Acked:    0,
		Unacked:  0,
		Errors:   1,
		Unknowns: 0,
	})
	results = reset()
}
