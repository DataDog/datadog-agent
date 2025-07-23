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
		return "", fmt.Errorf("empty digests list")
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
// This method checks if each image already exists in the workloadmeta store to avoid
// unnecessary expensive image status calls.
func (c *collector) generateImageEventsFromImageList(ctx context.Context) ([]workloadmeta.CollectorEvent, error) {
	images, err := c.client.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	log.Infof("[CRIO_OPTIMIZATION] Retrieved %d images from CRI-O ListImages call", len(images))

	imageEvents := make([]workloadmeta.CollectorEvent, 0)
	skippedCount := 0
	newImageCount := 0
	imageStatusCalls := 0

	for _, img := range images {
		// Extract the image ID - prefer digest over the raw ID
		imgID := img.GetId()
		if imgIDAsDigest, err := parseDigests(img.GetRepoDigests()); err == nil {
			imgID = imgIDAsDigest
		}

		// Check if image already exists in workloadmeta store
		// Try both computed ID and raw ID to handle ID format inconsistencies that can occur when:
		// 1. ListImages() returns digest info but GetContainerImage() returns empty digests (dangling images)
		// 2. Digest parsing fails differently between lookup and storage phases  
		// 3. Images are stored with raw ID due to empty repo digests but lookup computes digest ID
		// The fallback ensures we find existing images regardless of which ID format was used during storage
		rawID := img.GetId()
		var foundID string
		var usedRawIDFallback bool
		
		if _, err := c.store.GetImage(imgID); err == nil {
			foundID = imgID
			usedRawIDFallback = false
		} else if _, err := c.store.GetImage(rawID); err == nil {
			foundID = rawID
			usedRawIDFallback = true
		}
		
		if foundID != "" {
			// Create skipped image event to prevent it from being marked as removed
			skippedImageEvent := workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   foundID,
					},
				},
			}
			imageEvents = append(imageEvents, skippedImageEvent)
			skippedCount++
			
			if usedRawIDFallback {
				log.Infof("[CRIO_OPTIMIZATION] Skipping existing image %s (found via raw ID fallback: %s, computed ID: %s)", img.GetRepoTags(), rawID, imgID)
			} else {
				log.Debugf("[CRIO_OPTIMIZATION] Skipping existing image %s (ID: %s)", img.GetRepoTags(), imgID)
			}
			continue
		}

		// Image doesn't exist, need to get full metadata using image status
		newImageCount++
		imageStatusCalls++
		log.Infof("[CRIO_OPTIMIZATION] Making GetContainerImage call for new image %s (ID: %s, RawID: %s, RepoDigests: %s)", img.GetRepoTags(), imgID, img.GetId(), img.GetRepoDigests())
		
		imageSpec := &v1.ImageSpec{Image: img.GetId()}
		imageResp, err := c.client.GetContainerImage(ctx, imageSpec, true)
		if err != nil {
			log.Warnf("Failed to get image status for image %s: %v", img.GetId(), err)
			continue
		}

		// Log what we got back from GetContainerImage call
		respImg := imageResp.GetImage()
		log.Infof("[CRIO_OPTIMIZATION] GetContainerImage response - RepoTags: %s, RepoDigests: %s, ID: %s", respImg.GetRepoTags(), respImg.GetRepoDigests(), respImg.GetId())

		// Get namespace from any container using this image - use empty string as default
		namespace := ""

		imageEvent := c.convertImageToEvent(imageResp.GetImage(), imageResp.GetInfo(), namespace)
		imageEvents = append(imageEvents, *imageEvent)
	}

	log.Infof("[CRIO_OPTIMIZATION] Image collection summary: %d total images, %d skipped (optimization), %d new images processed, %d GetContainerImage calls made", 
		len(images), skippedCount, newImageCount, imageStatusCalls)

	return imageEvents, nil
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
