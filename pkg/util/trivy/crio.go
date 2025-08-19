// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && crio

// Package trivy holds the scan components
package trivy

import (
	"context"
	"fmt"
	"path/filepath"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type fakeCRIOContainer struct {
	*fakeContainer
}

func (c *fakeCRIOContainer) ID() (string, error) {
	return c.imgMeta.ID, nil
}

func (c *fakeCRIOContainer) ConfigFile() (*v1.ConfigFile, error) {
	configFile := &v1.ConfigFile{
		Architecture: c.imgMeta.Architecture,
		OS:           c.imgMeta.OS,
	}
	configFile.RootFS.DiffIDs = make([]v1.Hash, len(c.layerIDs))
	for i, diffID := range c.layerIDs {
		configFile.RootFS.DiffIDs[i], _ = v1.NewHash(diffID)
	}

	for _, layer := range c.imgMeta.Layers {
		configFile.History = append(configFile.History, v1.History{
			Author:     layer.History.Author,
			Created:    v1.Time{Time: *layer.History.Created},
			CreatedBy:  layer.History.CreatedBy,
			Comment:    layer.History.Comment,
			EmptyLayer: layer.History.EmptyLayer,
		})

	}
	return configFile, nil
}

func (c *fakeCRIOContainer) LayerByDiffID(hash string) (ftypes.LayerPath, error) {
	return c.fakeContainer.LayerByDiffID(hash)
}

func (c *fakeCRIOContainer) LayerByDigest(hash string) (ftypes.LayerPath, error) {
	return c.fakeContainer.LayerByDigest(hash)
}

func (c *fakeCRIOContainer) Layers() (layers []ftypes.LayerPath) {
	return c.fakeContainer.Layers()
}

func (c *fakeCRIOContainer) Name() string {
	return c.imgMeta.Name
}

func (c *fakeCRIOContainer) RepoTags() []string {
	return c.imgMeta.RepoTags
}

func (c *fakeCRIOContainer) RepoDigests() []string {
	return c.imgMeta.RepoDigests
}

// ScanCRIOImageFromOverlayFS scans the CRI-O image layers using OverlayFS.
func (c *Collector) ScanCRIOImageFromOverlayFS(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, client crio.Client, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	lowerDirs, err := client.GetCRIOImageLayers(imgMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve layer directories: %w", err)
	}

	diffIDs := make([]string, 0, len(lowerDirs))
	for _, dir := range lowerDirs {
		diffIDs = append(diffIDs, "sha256:"+filepath.Base(filepath.Dir(dir)))
	}

	report, err := c.scanOverlayFS(ctx, lowerDirs, &fakeCRIOContainer{
		fakeContainer: &fakeContainer{
			imgMeta:    imgMeta,
			layerPaths: lowerDirs,
			layerIDs:   diffIDs,
		},
	}, imgMeta, scanOptions)
	if err != nil {
		return nil, err
	}

	return report, nil
}
