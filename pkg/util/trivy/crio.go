// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && crio

// Package trivy holds the scan components
package trivy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/samber/lo"
)

// buildCRIOLayerPaths pairs each non-empty imgMeta.Layers entry with its CRI-O
// lowerDir and, when available, its per-layer manifest digest. DiffID is the
// image-config diff_id. manifestDigests holds the compressed-blob digests from
// the image manifest in image-config (base-to-top) order; they are paired
// positionally with the non-empty layers and left empty when their count does
// not match, mirroring the containerd overlayfs scan rather than risk a wrong
// pairing. GetCRIOImageLayers prepends each path while walking imgMeta.Layers in
// image-config order, so lowerDirs is top-to-base while the filtered layers stay
// base-to-top; walk lowerDirs in reverse to realign them.
func buildCRIOLayerPaths(imgMeta *workloadmeta.ContainerImageMetadata, lowerDirs, manifestDigests []string) ([]ftypes.LayerPath, error) {
	nonEmpty := lo.Filter(imgMeta.Layers, func(layer workloadmeta.ContainerImageLayer, _ int) bool {
		return layer.DiffID != ""
	})
	if len(nonEmpty) != len(lowerDirs) {
		return nil, fmt.Errorf("%w: %d lowerDirs vs %d non-empty imgMeta layers",
			errLayerCountMismatch, len(lowerDirs), len(nonEmpty))
	}
	n := len(lowerDirs)
	digestsAligned := len(manifestDigests) == n
	if len(manifestDigests) > 0 && !digestsAligned {
		log.Warnf("image %s: manifest has %d layer digests but %d non-empty layers; emitting SBOM without LayerDigest",
			imgMeta.ID, len(manifestDigests), n)
	}
	out := make([]ftypes.LayerPath, n)
	for i := 0; i < n; i++ {
		lp := ftypes.LayerPath{
			DiffID: nonEmpty[i].DiffID,
			Path:   lowerDirs[n-1-i],
		}
		if digestsAligned {
			lp.Digest = manifestDigests[i]
		}
		out[i] = lp
	}
	return out, nil
}

// readCRIOManifestLayerDigests reads the OCI image manifest CRI-O stores at
// overlay-images/<id>/manifest and returns each layer's compressed-blob digest
// in manifest (image-config, base-to-top) order. This is the same file the
// workloadmeta CRI-O collector reads for layer size and media type. It is best
// effort: it returns nil on any error so the caller emits an SBOM without
// LayerDigest rather than failing the scan.
func readCRIOManifestLayerDigests(imgID string) []string {
	id := strings.TrimPrefix(imgID, "sha256:")
	path := filepath.Join(crio.GetOverlayImagePath(), id, "manifest")
	file, err := os.Open(path)
	if err != nil {
		log.Debugf("failed to open CRI-O image manifest %s: %v", path, err)
		return nil
	}
	defer file.Close()

	var manifest struct {
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		log.Debugf("failed to decode CRI-O image manifest %s: %v", path, err)
		return nil
	}

	digests := make([]string, len(manifest.Layers))
	for i, layer := range manifest.Layers {
		digests[i] = layer.Digest
	}
	return digests
}

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
	diffIDs := c.diffIDs()
	configFile.RootFS.DiffIDs = make([]v1.Hash, len(diffIDs))
	for i, d := range diffIDs {
		configFile.RootFS.DiffIDs[i], _ = v1.NewHash(d)
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

	layers, err := buildCRIOLayerPaths(imgMeta, lowerDirs, readCRIOManifestLayerDigests(imgMeta.ID))
	if err != nil {
		return nil, err
	}
	report, err := c.scanOverlayFS(ctx, lowerDirs, &fakeCRIOContainer{
		fakeContainer: newFakeContainer(layers, imgMeta),
	}, imgMeta, scanOptions)
	if err != nil {
		return nil, err
	}

	return report, nil
}
