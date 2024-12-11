// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestIterableSeriesEmptyMarshalJSON(t *testing.T) {
	r := require.New(t)
	iterableSerie := CreateIterableSeries(CreateSerieSource(metrics.Series{}))
	bytes, err := iterableSerie.MarshalJSON()
	r.NoError(err)
	r.Equal(`{"series":[]}`, strings.TrimSpace(string(bytes)))
}

func TestIterableSeriesMoveNext(t *testing.T) {
	r := require.New(t)
	series := metrics.Series{
		&metrics.Serie{Name: "serie1", NoIndex: true},
		&metrics.Serie{Name: "serie2", NoIndex: false},
		&metrics.Serie{Name: "serie3", NoIndex: false},
		&metrics.Serie{Name: "serie4", NoIndex: true},
	}
	iterableSerie := CreateIterableSeries(CreateSerieSource(series))
	r.True(iterableSerie.MoveNext()) // Skip serie1
	r.True(strings.Contains(iterableSerie.DescribeCurrentItem(), "serie2"))
	r.True(iterableSerie.MoveNext())
	r.True(strings.Contains(iterableSerie.DescribeCurrentItem(), "serie3"))
	r.False(iterableSerie.MoveNext()) // Skip serie4
}
