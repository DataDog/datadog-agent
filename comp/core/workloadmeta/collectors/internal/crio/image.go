// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

package crio

import (
	"context"
	"encoding/json"
	"errors"
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
		return nil, fmt.Errorf("failed to retrieve image data for container %+v: %w", container, err)
	}
	image := imageResp.GetImage()

	namespace := getPodNamespace(ctx, c.client, container.GetPodSandboxId())

	imageEvent := c.convertImageToEvent(image, imageResp.GetInfo(), namespace)
	return imageEvent, nil
}

// convertImageToEvent converts a CRI-O image and additional metadata into a workloadmeta CollectorEvent.
func (c *collector) convertImageToEvent(img *v1.Image, info map[string]string, namespace string) *workloadmeta.CollectorEvent {

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
	imgID := img.GetId()
	imgInfo := parseImageInfo(info, crio.GetOverlayImagePath(), imgID)

	imgIDAsDigest, err := parseDigests(img.GetRepoDigests())
	if err == nil {
		imgID = imgIDAsDigest
	} else if sbomCollectionIsEnabled() {
		log.Warnf("Failed to parse digest for image with ID %s: %v. As a result, SBOM vulnerabilities may not be properly linked to this image.", imgID, err)
	}

	imgMeta := workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imgID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
			Labels:      imgInfo.labels,
		},
		SizeBytes:    imgInfo.size,
		RepoTags:     img.GetRepoTags(),
		RepoDigests:  img.GetRepoDigests(),
		OS:           imgInfo.os,
		Architecture: imgInfo.arch,
		Variant:      imgInfo.variant,
		Layers:       imgInfo.layers,
	}

	return &workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceRuntime,
		Entity: &imgMeta,
	}
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
		return "", errors.New("empty digests list")
	}
	parts := strings.SplitN(imageRefs[0], "@", 2)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid format: no digest found in %s", imageRefs[0])
	}

	return parts[1], nil
}

// parseImageInfo extracts operating system, architecture, variant, labels, and layer history from image info metadata.
func parseImageInfo(info map[string]string, layerFilePath string, imgID string) imageInfo {
	var imgInfo imageInfo

	// Fetch additional layer information from the file
	layerDetails, err := parseLayerInfo(layerFilePath, imgID)
	if err != nil {
		log.Debugf("Failed to get layer mediaType and size: %v", err)
	}

	if imgSpec, ok := info["info"]; ok {
		var parsed parsedInfo

		if err := json.Unmarshal([]byte(imgSpec), &parsed); err == nil {
			imgInfo.os = parsed.ImageSpec.OS
			imgInfo.arch = parsed.ImageSpec.Architecture
			imgInfo.variant = parsed.ImageSpec.Variant
			imgInfo.labels = parsed.Labels

			// Match layers with their history entries, including empty layers
			historyIndex := 0
			for layerIndex, layerDigest := range parsed.ImageSpec.RootFS.DiffIDs {
				// Append all empty layers encountered before this layer
				for historyIndex < len(parsed.ImageSpec.History) {
					history := parsed.ImageSpec.History[historyIndex]
					if history.EmptyLayer {
						created, _ := time.Parse(time.RFC3339, history.Created)
						imgInfo.layers = append(imgInfo.layers, workloadmeta.ContainerImageLayer{
							History: &imgspecs.History{
								Created:    &created,
								CreatedBy:  history.CreatedBy,
								Author:     history.Author,
								Comment:    history.Comment,
								EmptyLayer: history.EmptyLayer,
							},
						})
						historyIndex++
					} else {
						// Stop at the first non-empty layer
						break
					}
				}

				// Match the non-empty history to this layer
				var historyEntry *imgspecs.History
				if historyIndex < len(parsed.ImageSpec.History) {
					h := parsed.ImageSpec.History[historyIndex]
					created, _ := time.Parse(time.RFC3339, h.Created)
					historyEntry = &imgspecs.History{
						Created:    &created,
						CreatedBy:  h.CreatedBy,
						Author:     h.Author,
						Comment:    h.Comment,
						EmptyLayer: h.EmptyLayer,
					}
					historyIndex++
				}

				// Create and append the layer with the matched history
				layer := workloadmeta.ContainerImageLayer{
					Digest:  layerDigest,
					History: historyEntry,
				}

				// Add additional details from parsed layer info
				if layerIndex < len(layerDetails) {
					imgInfo.size += int64(layerDetails[layerIndex].Size)
					layer.SizeBytes = int64(layerDetails[layerIndex].Size)
					layer.MediaType = layerDetails[layerIndex].MediaType
				}

				imgInfo.layers = append(imgInfo.layers, layer)
			}

			// Append any remaining empty layers
			for historyIndex < len(parsed.ImageSpec.History) {
				history := parsed.ImageSpec.History[historyIndex]
				if history.EmptyLayer {
					created, _ := time.Parse(time.RFC3339, history.Created)
					imgInfo.layers = append(imgInfo.layers, workloadmeta.ContainerImageLayer{
						History: &imgspecs.History{
							Created:    &created,
							CreatedBy:  history.CreatedBy,
							Author:     history.Author,
							Comment:    history.Comment,
							EmptyLayer: history.EmptyLayer,
						},
					})
				}
				historyIndex++
			}
		} else {
			log.Warnf("Failed to parse image info: %v", err)
		}
	}

	return imgInfo
}

// generateImageEventsFromImageList creates workloadmeta image events from the full image list.
// Returns events for new images and IDs of all current images (for seenImages tracking).
func (c *collector) generateImageEventsFromImageList(ctx context.Context) ([]workloadmeta.CollectorEvent, []workloadmeta.EntityID, error) {
	images, err := c.client.ListImages(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list images: %w", err)
	}

	imageEvents := make([]workloadmeta.CollectorEvent, 0)
	allImageIDs := make([]workloadmeta.EntityID, 0, len(images))

	for _, img := range images {
		// Extract the image ID - prefer digest over the raw ID (same logic as convertImageToEvent)
		rawID := img.GetId()
		imgID := rawID
		if imgIDAsDigest, err := parseDigests(img.GetRepoDigests()); err == nil {
			imgID = imgIDAsDigest
		}

		// Always track with the computed ID (same as what convertImageToEvent would use)
		entityID := workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: imgID}
		allImageIDs = append(allImageIDs, entityID)

		// Check if image already exists in workloadmeta store
		// Try computed ID first, then raw ID as fallback for edge cases
		var existsInStore bool
		if _, err := c.store.GetImage(imgID); err == nil {
			existsInStore = true
		} else if imgID != rawID {
			// Only try raw ID if it's different from computed ID
			if _, err := c.store.GetImage(rawID); err == nil {
				existsInStore = true
			}
		}

		if existsInStore {
			// Image already exists in store - no need to send event, metadata persists automatically
			continue
		}

		imageSpec := &v1.ImageSpec{Image: img.GetId()}
		imageResp, err := c.client.GetContainerImage(ctx, imageSpec, true)
		if err != nil {
			log.Warnf("Failed to get image status for image %s: %v", img.GetId(), err)
			continue
		}

		// Get namespace from any container using this image - use empty string as default
		namespace := ""

		imageEvent := c.convertImageToEvent(imageResp.GetImage(), imageResp.GetInfo(), namespace)
		imageEvents = append(imageEvents, *imageEvent)
	}

	return imageEvents, allImageIDs, nil
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

// imageInfo holds the size, OS, architecture, variant, labels, and layers of an image.
type imageInfo struct {
	size    int64
	os      string
	arch    string
	variant string
	labels  map[string]string
	layers  []workloadmeta.ContainerImageLayer
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
