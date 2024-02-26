// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/opencontainers/go-digest"
	ociSpec "github.com/opencontainers/image-spec/specs-go/v1"
)

// extractOCI extracts the OCI archive at `ociArchivePath`
func extractOCI(ociArchivePath string, destinationPath string) error {
	index := &ociSpec.Index{}
	indexFile, err := os.Open(path.Join(ociArchivePath, "index.json"))
	if err != nil {
		return fmt.Errorf("could not open index file: %w", err)
	}
	err = json.NewDecoder(indexFile).Decode(index)
	if err != nil {
		return fmt.Errorf("could not parse index file: %w", err)
	}

	for _, manifest := range index.Manifests {
		err := extractOCIManifest(ociArchivePath, destinationPath, manifest)
		if err != nil {
			return err // already wrapped
		}
	}
	return nil
}

// extractOCIManifest extracts the layers of a single manifest from the OCI archive
func extractOCIManifest(ociArchivePath string, destinationPath string, manifest ociSpec.Descriptor) error {
	if manifest.Digest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("invalid algorithm %s for manifest: only sha256 is supported", manifest.Digest.Algorithm())
	}
	blobsPath := path.Join(ociArchivePath, "blobs", string(manifest.Digest.Algorithm()))
	err := verifyOCIFile(blobsPath, manifest)
	if err != nil {
		return err // already wrapped
	}

	manifestStruct := &ociSpec.Manifest{}
	manifestFile, err := os.Open(path.Join(blobsPath, manifest.Digest.Encoded()))
	if err != nil {
		return fmt.Errorf("could not open manifest file: %w", err)
	}
	err = json.NewDecoder(manifestFile).Decode(manifestStruct)
	if err != nil {
		return fmt.Errorf("could not parse manifest file: %w", err)
	}

	for _, layer := range manifestStruct.Layers {
		err := extractOCILayer(blobsPath, destinationPath, layer)
		if err != nil {
			return err // already wrapped
		}
	}
	return nil
}

// extractOCILayer extracts & verifies a layer from the OCI archive to `destinationPath“
func extractOCILayer(blobsPath string, destinationPath string, layer ociSpec.Descriptor) error {
	if layer.Digest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("invalid algorithm %s for layer: only sha256 is supported", layer.Digest.Algorithm())
	}
	err := verifyOCIFile(blobsPath, layer)
	if err != nil {
		return err // already wrapped
	}

	layerPath := path.Join(blobsPath, layer.Digest.Encoded())
	switch layer.MediaType {
	case ociSpec.MediaTypeImageLayerGzip:
		err := extractTarArchive(layerPath, destinationPath, compressionGzip)
		if err != nil {
			return err // already wrapped
		}
	default:
		return fmt.Errorf("unsupported media type %s for layer", layer.MediaType)
	}
	return nil
}

// verifyOCIFile verifies the length & digest of a file in the OCI archive given its descriptor
func verifyOCIFile(blobsPath string, spec ociSpec.Descriptor) error {
	filePath := path.Join(blobsPath, spec.Digest.Encoded())
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	verifier := spec.Digest.Verifier()
	n, err := io.Copy(verifier, file)
	if err != nil {
		return fmt.Errorf("could not write file to verifier: %w", err)
	}
	if n != spec.Size {
		return fmt.Errorf("invalid size for file: expected %d, got %d", spec.Size, n)
	}
	if !verifier.Verified() {
		return fmt.Errorf("invalid hash for file: %w", err)
	}
	return nil
}
