// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy
// +build trivy

package trivy

import (
	"testing"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/stretchr/testify/require"
)

func TestBoltCache_Artifacts(t *testing.T) {
	cache, err := NewBoltCache(t.TempDir())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cache.Close())
	}()

	_, err = cache.GetArtifact("non-existing-ID")
	require.Error(t, err)

	artifactID := "some_ID"
	artifactInfo := newTestArtifactInfo()

	err = cache.PutArtifact(artifactID, artifactInfo)
	require.NoError(t, err)

	storedArtifact, err := cache.GetArtifact(artifactID)
	require.NoError(t, err)
	require.Equal(t, artifactInfo, storedArtifact)
}

func newTestArtifactInfo() types.ArtifactInfo {
	return types.ArtifactInfo{
		SchemaVersion: 1,
		Architecture:  "amd64",
		Created:       time.Date(2023, 2, 28, 0, 0, 0, 0, time.UTC),
		DockerVersion: "18.06.1-ce",
		OS:            "linux",
		HistoryPackages: []types.Package{
			{
				Name:    "musl",
				Version: "1.2.3",
			},
		},
	}
}

func newTestBlobInfo() types.BlobInfo {
	return types.BlobInfo{
		SchemaVersion: 1,
		OS: types.OS{
			Family: "alpine",
			Name:   "3.17",
		},
	}
}
