// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestSerieRowFromSerieNormalizesSpecialTagsWithoutMutatingSerie(t *testing.T) {
	serie := &Serie{
		Name:      "system.net.bytes",
		Points:    []Point{{Ts: 1, Value: 2}},
		Tags:      tagset.CompositeTagsFromSlice([]string{"env:prod", "device:eth0", "dd.internal.resource:pod:api", "dd.internal.resource:bad", "zone:us"}),
		Host:      "host-a",
		Device:    "original-device",
		MType:     APIGaugeType,
		Resources: []Resource{{Type: "container", Name: "abc"}},
		Source:    MetricSourceDogstatsd,
	}

	row := SerieRowFromSerie(serie)

	assert.Equal(t, "eth0", row.Device)
	assert.Equal(t, tagset.CompositeTagsFromSlice([]string{"env:prod", "zone:us"}), row.Tags)
	require.Len(t, row.Resources, 2)
	assert.Equal(t, Resource{Type: "container", Name: "abc"}, row.Resources[0])
	assert.Equal(t, Resource{Type: "pod", Name: "api"}, row.Resources[1])

	// The shared Serie remains untouched so non-row payload variants preserve the
	// existing compatibility path.
	assert.Equal(t, "original-device", serie.Device)
	assert.Equal(t, tagset.CompositeTagsFromSlice([]string{"env:prod", "device:eth0", "dd.internal.resource:pod:api", "dd.internal.resource:bad", "zone:us"}), serie.Tags)
	assert.Equal(t, []Resource{{Type: "container", Name: "abc"}}, serie.Resources)
}
