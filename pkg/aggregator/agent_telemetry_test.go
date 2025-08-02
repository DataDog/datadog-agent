// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/assert"
)

func TestGetDetailsFromSerie(t *testing.T) {
	serie1 := metrics.Serie{
		Name:   "datadog.dogstatsd.client.bytes_sent",
		Points: []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:   tagset.CompositeTagsFromSlice([]string{"client:ruby", "client_version:5.4.2", "client_transport:udp"}),
		Host:   "",
		MType:  metrics.APICountType,
	}

	value, tags := getDetailsFromSerie(&serie1)

	assert.Equal(t, 1.0, value)
	assert.Equal(t, tags, []string{"ruby", "5.4.2", "udp"})
}

func TestGetDetailsFromSerieUnorderedTags(t *testing.T) {
	serie1 := metrics.Serie{
		Name:   "datadog.dogstatsd.client.bytes_sent",
		Points: []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:   tagset.CompositeTagsFromSlice([]string{"client_transport:udp", "client_version:5.4.2", "client:ruby"}),
		Host:   "",
		MType:  metrics.APICountType,
	}

	value, tags := getDetailsFromSerie(&serie1)

	assert.Equal(t, 1.0, value)
	assert.Equal(t, tags, []string{"ruby", "5.4.2", "udp"})
}

func TestGetDetailsFromSerieMissingTags(t *testing.T) {
	serie1 := metrics.Serie{
		Name:   "datadog.dogstatsd.client.bytes_sent",
		Points: []metrics.Point{{Ts: 12340.0, Value: float64(543)}},
		Tags:   tagset.CompositeTagsFromSlice([]string{"client:ruby", "zork:32", "client_transport:udp"}),
		Host:   "",
		MType:  metrics.APICountType,
	}

	value, tags := getDetailsFromSerie(&serie1)

	assert.Equal(t, 543.0, value)
	assert.Equal(t, tags, []string{"ruby", "", "udp"})
}

func TestGetDetailsFromSerieMultiplePoints(t *testing.T) {
	serie1 := metrics.Serie{
		Name: "datadog.dogstatsd.client.bytes_sent",
		Points: []metrics.Point{
			{Ts: 12340.0, Value: float64(1)},
			{Ts: 12340.0, Value: float64(423)},
			{Ts: 12340.0, Value: float64(8234)},
		},
		Tags:  tagset.CompositeTagsFromSlice([]string{"client_transport:udp", "client_version:5.4.2", "client:ruby"}),
		Host:  "",
		MType: metrics.APICountType,
	}

	value, tags := getDetailsFromSerie(&serie1)

	assert.Equal(t, 8658.0, value)
	assert.Equal(t, tags, []string{"ruby", "5.4.2", "udp"})
}
