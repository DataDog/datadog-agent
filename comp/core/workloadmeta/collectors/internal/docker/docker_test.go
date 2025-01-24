// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func Test_LayersFromDockerHistoryAndInspect(t *testing.T) {
	var emptySize int64
	var noDiffCmd = "ENV var=dummy"

	var nonEmptySize int64 = 1
	var cmd = "RUN /bin/sh dummy"

	var baseTimeUnix int64
	var baseTime = time.Unix(baseTimeUnix, 0)

	var layerID = "dummy id"

	tests := []struct {
		name     string
		history  []image.HistoryResponseItem
		inspect  types.ImageInspect
		expected []workloadmeta.ContainerImageLayer
	}{
		{
			name: "Equal number of history and inspect layers produces expected result",
			history: []image.HistoryResponseItem{
				{
					Size:      nonEmptySize,
					CreatedBy: cmd,
					Created:   baseTimeUnix,
				},
			},
			inspect: types.ImageInspect{
				RootFS: types.RootFS{
					Layers: []string{layerID},
				},
			},
			expected: []workloadmeta.ContainerImageLayer{
				{
					Digest:    layerID,
					SizeBytes: nonEmptySize,
					History: &v1.History{
						Created:    &baseTime,
						CreatedBy:  cmd,
						EmptyLayer: false,
					},
				},
			},
		},
		{
			name: "History with empty layer merges correctly with inspect layers and produces expected result",
			history: []image.HistoryResponseItem{
				{
					Size:      nonEmptySize,
					CreatedBy: cmd,
					Created:   baseTimeUnix,
				},
				// history is reverse-chronological so we will expect this to be the first layer processed
				{
					Size:      emptySize,
					CreatedBy: noDiffCmd,
					Created:   baseTimeUnix,
				},
			},
			inspect: types.ImageInspect{
				RootFS: types.RootFS{
					Layers: []string{layerID},
				},
			},
			expected: []workloadmeta.ContainerImageLayer{
				{
					Digest:    "", // should be empty because first history layer is empty
					SizeBytes: emptySize,
					History: &v1.History{
						Created:    &baseTime,
						CreatedBy:  noDiffCmd,
						EmptyLayer: true,
					},
				},
				{
					Digest:    layerID,
					SizeBytes: nonEmptySize,
					History: &v1.History{
						Created:    &baseTime,
						CreatedBy:  cmd,
						EmptyLayer: false,
					},
				},
			},
		},
		{
			name: "Number of inspect layers exceeds history layers breaks our assumption and results in no layers returned",
			history: []image.HistoryResponseItem{
				{
					Size:      nonEmptySize,
					CreatedBy: cmd,
					Created:   baseTimeUnix,
				},
			},
			inspect: types.ImageInspect{
				RootFS: types.RootFS{
					Layers: []string{layerID, layerID}, // multiple layers here
				},
			},
			expected: []workloadmeta.ContainerImageLayer{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layers := layersFromDockerHistoryAndInspect(tt.history, tt.inspect)
			assert.ElementsMatchf(t, tt.expected, layers, "Expected layers and actual layers returned do not match")
		})
	}
}
