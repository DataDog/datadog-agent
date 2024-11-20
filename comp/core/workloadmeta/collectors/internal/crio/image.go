// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	imgspecs "github.com/opencontainers/image-spec/specs-go/v1"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// generateImageEventFromContainer creates a workloadmeta image event based on container image metadata.
func (c *collector) generateImageEventFromContainer(ctx context.Context, container *v1.Container) (*workloadmeta.CollectorEvent, error) {
	if container.GetImage() == nil || container.GetImage().GetImage() == "" {
		return nil, fmt.Errorf("container has an invalid image reference: %+v", container)
	}
	imageSpec := v1.ImageSpec{Image: container.GetImage().GetImage()}
	imageResp, err := c.client.GetContainerImage(ctx, &imageSpec, true)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve image data for container %+v: %v", container, err)
	}
	image := imageResp.GetImage()

	namespace := getPodNamespace(ctx, c.client, container.GetPodSandboxId())

	imageEvent := c.convertImageToEvent(image, imageResp.GetInfo(), namespace, nil)
	return imageEvent, nil
}

// convertImageToEvent converts a CRI-O image and additional metadata into a workloadmeta CollectorEvent.
func (c *collector) convertImageToEvent(img *v1.Image, info map[string]string, namespace string, sbom *workloadmeta.SBOM) *workloadmeta.CollectorEvent {
	var annotations map[string]string
	if img.GetSpec() == nil {
		annotations = nil
	} else {
		annotations = img.GetSpec().GetAnnotations()
	}

	var name string
	if len(img.GetRepoTags()) > 0 {
		name = img.GetRepoTags()[0]
	}
	imgId := img.GetId()
	os, arch, variant, labels, layers := parseImageInfo(info, crio.GetOverlayImagePath(), imgId)

	imgIdAsDigest, err := parseDigests(img.GetRepoDigests())
	if err == nil {
		imgId = imgIdAsDigest
	} else if sbomCollectionIsEnabled() {
		log.Warnf("Failed to parse digest for image with ID %s: %v. As a result, SBOM vulnerabilities may not be properly linked to this image.", imgId, err)
	}

	imgMeta := workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imgId,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		SizeBytes:    int64(img.Size_),
		RepoTags:     img.GetRepoTags(),
		RepoDigests:  img.GetRepoDigests(),
		SBOM:         sbom,
		OS:           os,
		Architecture: arch,
		Variant:      variant,
		Layers:       layers,
	}

	return &workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceRuntime,
		Entity: &imgMeta,
	}
}

// notifyStoreWithSBOMForImage notifies the store about the SBOM for a given image.
func (c *collector) notifyStoreWithSBOMForImage(imageID string, sbom *workloadmeta.SBOM) {
	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceTrivy,
			Entity: &workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   imageID,
				},
				SBOM: sbom,
			},
		},
	})
}

// generateUnsetImageEvent generates an unset CollectorEvent for a removed or deleted image.
func generateUnsetImageEvent(seenID workloadmeta.EntityID) *workloadmeta.CollectorEvent {
	return &workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.ContainerImageMetadata{
			EntityID: seenID,
		},
	}
}

// parseDigests extracts the SHA from the image reference digest.
// The backend requires the image ID to be set as the SHA to correctly associate the SBOM with the image.
func parseDigests(imageRefs []string) (string, error) {
	if len(imageRefs) == 0 {
		return "", fmt.Errorf("empty digests list")
	}
	parts := strings.SplitN(imageRefs[0], "@", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid format: no digest found in %s", imageRefs[0])
	}

	return parts[1], nil
}

// parseImageInfo extracts operating system, architecture, variant, labels, and layer history from image info metadata.
func parseImageInfo(info map[string]string, layerFilePath string, imgID string) (string, string, string, map[string]string, []workloadmeta.ContainerImageLayer) {
	var os, arch, variant string
	var layers []workloadmeta.ContainerImageLayer
	var labels map[string]string

	// Fetch additional layer information from the file
	layerDetails, err := parseLayerInfo(layerFilePath, imgID)
	if err != nil {
		log.Debugf("Failed to get layer mediaType and size: %v", err)
	}

	if imgSpec, ok := info["info"]; ok {
		var parsed parsedInfo

		if err := json.Unmarshal([]byte(imgSpec), &parsed); err == nil {
			os = parsed.ImageSpec.OS
			arch = parsed.ImageSpec.Architecture
			variant = parsed.ImageSpec.Variant
			labels = parsed.Labels

			// Match layers with their history entries, ignoring empty layers
			historyIndex := 0
			for i, layerDigest := range parsed.ImageSpec.RootFS.DiffIDs {
				var historyEntry *imgspecs.History

				// Loop until we find a non-empty history layer entry that corresponds to a layer
				for historyIndex < len(parsed.ImageSpec.History) {
					h := parsed.ImageSpec.History[historyIndex]
					historyIndex++
					if h.EmptyLayer {
						continue
					}

					created, _ := time.Parse(time.RFC3339, h.Created)
					historyEntry = &imgspecs.History{
						Created:    &created,
						CreatedBy:  h.CreatedBy,
						Author:     h.Author,
						Comment:    h.Comment,
						EmptyLayer: h.EmptyLayer,
					}
					break
				}

				layer := workloadmeta.ContainerImageLayer{
					Digest:  layerDigest,
					History: historyEntry,
				}

				if i < len(layerDetails) {
					layer.SizeBytes = int64(layerDetails[i].Size)
					layer.MediaType = layerDetails[i].MediaType
				}

				layers = append(layers, layer)
			}
		} else {
			log.Warnf("Failed to parse image info: %v", err)
		}
	}

	return os, arch, variant, labels, layers
}

// parseLayerInfo reads a JSON file from the given path and returns a list of layerInfo
func parseLayerInfo(rootPath string, imgID string) ([]layerInfo, error) {
	filePath := fmt.Sprintf("%s/%s/manifest", rootPath, imgID)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var manifest struct {
		Layers []layerInfo `json:"layers"`
	}

	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return manifest.Layers, nil
}

// layerInfo holds the size and mediaType of each layer
type layerInfo struct {
	Size      int    `json:"size"`
	MediaType string `json:"mediaType"`
}

// parsedInfo holds layer metadata extracted from image JSON.
type parsedInfo struct {
	Labels    map[string]string `json:"labels"`
	ImageSpec struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
		RootFS       struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
		History []struct {
			Created    string `json:"created"`
			CreatedBy  string `json:"created_by"`
			Author     string `json:"author"`
			Comment    string `json:"comment"`
			EmptyLayer bool   `json:"empty_layer"`
		} `json:"history"`
	} `json:"imageSpec"`
}
