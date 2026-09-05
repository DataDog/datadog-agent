// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerendpointimpl

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contexttop"
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

func TestGetDogstatsdTop(t *testing.T) {
	endpoint := demultiplexerEndpoint{
		demux: fakeContextDumper{
			{Name: "requests", MetricTags: []string{"env:prod", "endpoint:/a"}},
			{Name: "requests", MetricTags: []string{"env:prod", "endpoint:/b"}},
		},
		runPath: t.TempDir(),
	}

	result, err := endpoint.getDogstatsdTop(10, 5)
	require.NoError(t, err)
	require.Equal(t, contexttop.Result{
		Metrics: []contexttop.Metric{
			{
				Name:     "requests",
				Contexts: 2,
				Tags: []contexttop.Tag{
					{Key: "endpoint", UniqueValues: 2},
					{Key: "env", UniqueValues: 1},
				},
			},
		},
	}, result)
}

func TestValidateTopRequest(t *testing.T) {
	for _, request := range []topRequest{
		{NumMetrics: 0, NumTags: 5},
		{NumMetrics: maxNumMetrics + 1, NumTags: 5},
		{NumMetrics: 10, NumTags: 0},
		{NumMetrics: 10, NumTags: maxNumTags + 1},
	} {
		require.Error(t, validateTopRequest(request))
	}

	require.NoError(t, validateTopRequest(topRequest{NumMetrics: 10, NumTags: 5}))
}
