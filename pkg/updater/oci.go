package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/opencontainers/go-digest"
	ociSpec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	MediaTypeImageLayerXz = "application/vnd.oci.image.layer.xz"
)

// extractOCI extracts the OCI archive at `ociArchivePath`
func extractOCI(ociArchivePath string, destinationPath string) error {
	index := &ociSpec.Index{}
	indexFile, err := os.ReadFile(path.Join(ociArchivePath, "index.json"))
	if err != nil {
		return fmt.Errorf("could not open index file: %w", err)
	}
	err = json.Unmarshal(indexFile, index)
	if err != nil {
		return fmt.Errorf("could not parse index file: %w", err)
	}

	for _, manifest := range index.Manifests {
		if err := extractOCIManifest(ociArchivePath, destinationPath, manifest); err != nil {
			return fmt.Errorf("could not extract manifest: %w", err)
		}
	}
	return nil
}

// extractOCIManifest extracts the layers of a single manifest from the OCI archive
func extractOCIManifest(ociArchivePath string, destinationPath string, manifest ociSpec.Descriptor) error {
	if manifest.Digest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("invalid algorithm %s for manifest: only sha256 is supported", manifest.Digest.Algorithm())
	}

	// Read manifest file
	blobsPath := path.Join(ociArchivePath, "blobs", string(manifest.Digest.Algorithm()))
	manifestFile, err := os.ReadFile(path.Join(blobsPath, manifest.Digest.Encoded()))
	if err != nil {
		return fmt.Errorf("could not open manifest file: %w", err)
	}

	// Verify length & digest of the manifest
	if manifest.Size != int64(len(manifestFile)) {
		return fmt.Errorf("invalid size for manifest: expected %d, got %d", manifest.Size, len(manifestFile))
	}
	verifier := manifest.Digest.Verifier()
	_, err = verifier.Write(manifestFile)
	if err != nil {
		return fmt.Errorf("could not write manifest to verifier: %w", err)
	}
	if !verifier.Verified() {
		return fmt.Errorf("invalid hash for manifest: %w", err)
	}

	// Parse manifest
	manifestStruct := &ociSpec.Manifest{}
	err = json.Unmarshal(manifestFile, manifestStruct)
	if err != nil {
		return fmt.Errorf("could not parse manifest file: %w", err)
	}

	// Extract layers in the destination folder
	for _, layer := range manifestStruct.Layers {
		if err := extractOCILayer(blobsPath, destinationPath, layer); err != nil {
			return fmt.Errorf("could not extract layer: %w", err)
		}
	}

	return nil
}

// extractOCILayer extracts & verifies a layer from the OCI archive to `destinationPath“
//
// Note: we could add the manifest configuration to this method, but today it is not necessary
// as there is no additional information required for the extraction in it.
func extractOCILayer(blobsPath string, destinationPath string, layer ociSpec.Descriptor) error {
	if layer.Digest.Algorithm() != digest.SHA256 {
		return fmt.Errorf("invalid algorithm %s for layer: only sha256 is supported", layer.Digest.Algorithm())
	}

	// Read layer file
	layerPath := path.Join(blobsPath, layer.Digest.Encoded())
	layerFile, err := os.ReadFile(layerPath)
	if err != nil {
		return fmt.Errorf("could not open layer file: %w", err)
	}

	// Verify length & digest of the layer
	if layer.Size != int64(len(layerFile)) {
		return fmt.Errorf("invalid size for layer: expected %d, got %d", layer.Size, len(layerFile))
	}
	verifier := layer.Digest.Verifier()
	_, err = verifier.Write(layerFile)
	if err != nil {
		return fmt.Errorf("could not write layer to verifier: %w", err)
	}
	if !verifier.Verified() {
		return fmt.Errorf("invalid hash for layer: %w", err)
	}

	switch layer.MediaType {
	case MediaTypeImageLayerXz, "application/vnd.oci.image.layer.v1.tar+zstd": // ZSTD is for testing purposes as XZ archives wrongly declare ZSTD
		if err := extractTarXz(layerPath, destinationPath); err != nil {
			return fmt.Errorf("could not extract layer: %w", err)
		}
	default:
		return fmt.Errorf("unsupported media type %s for layer", layer.MediaType)
	}

	return nil
}
