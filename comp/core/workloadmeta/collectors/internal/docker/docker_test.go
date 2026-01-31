// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func Test_LayersFromDockerHistoryAndInspect(t *testing.T) {
	var emptySize int64
	var noDiffCmd = "ENV var=dummy"

	var nonEmptySize int64 = 1
	var cmd = "COPY dummy.sh ."

	var baseTimeUnix int64
	var baseTime = time.Unix(baseTimeUnix, 0)

	var layerID = "dummy id"

	tests := []struct {
		name     string
		history  []image.HistoryResponseItem
		inspect  image.InspectResponse
		expected []workloadmeta.ContainerImageLayer
	}{
		{
			name: "Layer with CreatedBy and positive Size is assigned a digest",
			history: []image.HistoryResponseItem{
				{
					Size:      nonEmptySize,
					CreatedBy: cmd,
					Created:   baseTimeUnix,
				},
			},
			inspect: image.InspectResponse{
				RootFS: image.RootFS{
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
			name: "Inherited layer with no CreatedBy and no Size is detected and is assigned a digest",
			history: []image.HistoryResponseItem{
				{
					Size:    emptySize,
					Created: baseTimeUnix,
				},
			},
			inspect: image.InspectResponse{
				RootFS: image.RootFS{
					Layers: []string{layerID},
				},
			},
			expected: []workloadmeta.ContainerImageLayer{
				{
					Digest:    layerID,
					SizeBytes: emptySize,
					History: &v1.History{
						Created:    &baseTime,
						EmptyLayer: true,
					},
				},
			},
		},
		{
			name: "Layer with CreatedBy and empty Size is NOT assigned a digest",
			history: []image.HistoryResponseItem{
				{
					Size:      emptySize,
					CreatedBy: noDiffCmd,
					Created:   baseTimeUnix,
				},
			},
			inspect: image.InspectResponse{
				RootFS: image.RootFS{
					Layers: []string{layerID},
				},
			},
			expected: []workloadmeta.ContainerImageLayer{
				{
					SizeBytes: emptySize,
					History: &v1.History{
						CreatedBy:  noDiffCmd,
						Created:    &baseTime,
						EmptyLayer: true,
					},
				},
			},
		},
		{
			name: "Mix of layers with and without digests are merged in the proper order",
			history: []image.HistoryResponseItem{
				{ // "2" in the expected field
					Size:      nonEmptySize,
					Created:   baseTimeUnix,
					CreatedBy: cmd,
				},
				{
					Size:      emptySize,
					Created:   baseTimeUnix,
					CreatedBy: noDiffCmd,
				},
				{ // "1" in the expected field
					Size:    emptySize,
					Created: baseTimeUnix,
				},
			},
			inspect: image.InspectResponse{
				RootFS: image.RootFS{
					Layers: []string{"1", "2"},
				},
			},
			expected: []workloadmeta.ContainerImageLayer{
				{
					Digest:    "1",
					SizeBytes: emptySize,
					History: &v1.History{
						Created:    &baseTime,
						EmptyLayer: true,
					},
				},
				{
					SizeBytes: emptySize,
					History: &v1.History{
						Created:    &baseTime,
						CreatedBy:  noDiffCmd,
						EmptyLayer: true,
					},
				},
				{
					Digest:    "2",
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
			name: "Number of assignable history layers exceeds inspect layers does not result in panic",
			history: []image.HistoryResponseItem{
				{
					Size:      nonEmptySize,
					CreatedBy: cmd,
					Created:   baseTimeUnix,
				},
				{
					Size:      nonEmptySize,
					CreatedBy: cmd,
					Created:   baseTimeUnix,
					ID:        "abc",
				},
			},
			inspect: image.InspectResponse{
				RootFS: image.RootFS{
					Layers: []string{"1"},
				},
			},
			expected: []workloadmeta.ContainerImageLayer{
				{
					Digest:    "",
					SizeBytes: nonEmptySize,
					History: &v1.History{
						Created:    &baseTime,
						CreatedBy:  cmd,
						EmptyLayer: false,
					},
				},
				{
					Digest:    "abc",
					SizeBytes: nonEmptySize,
					History: &v1.History{
						Created:    &baseTime,
						CreatedBy:  cmd,
						EmptyLayer: false,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layers := layersFromDockerHistoryAndInspect(tt.history, tt.inspect)
			assert.ElementsMatchf(t, tt.expected, layers, "Expected layers and actual layers returned do not match")
		})
	}
}
