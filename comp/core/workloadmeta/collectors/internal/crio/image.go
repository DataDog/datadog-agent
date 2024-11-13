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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"os"
	"time"

	imgspecs "github.com/opencontainers/image-spec/specs-go/v1"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/crio"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// imageMetadataCollectionIsEnabled checks if image metadata collection is enabled via configuration.
func imageMetadataCollectionIsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("container_image.enabled")
}

// generateImageEventFromContainer creates a workloadmeta image event based on container image data.
// It retrieves detailed image metadata and creates a CollectorEvent with full metadata including SBOM status.
func (c *collector) generateImageEventFromContainer(ctx context.Context, container *v1.Container) workloadmeta.CollectorEvent {
	imageSpec := v1.ImageSpec{Image: container.Image.Image}
	imageResp, err := c.client.GetContainerImage(ctx, &imageSpec, true)
	if err != nil {
		log.Warnf("Failed to retrieve image data for spec %v: %v", imageSpec.Image, err)
		return workloadmeta.CollectorEvent{Type: workloadmeta.EventTypeUnset}
	}
	image := imageResp.Image

	namespace := getPodNamespace(ctx, c.client, container.PodSandboxId)

	imageEvent := c.convertImageToEvent(image, imageResp.Info, namespace, &workloadmeta.SBOM{Status: workloadmeta.Pending})
	return imageEvent
}

// convertImageToEvent converts a CRI-O image and additional metadata into a workloadmeta CollectorEvent.
// It includes OS, architecture, labels, annotations, and layer history to create a comprehensive metadata event.
func (c *collector) convertImageToEvent(img *v1.Image, info map[string]string, namespace string, sbom *workloadmeta.SBOM) workloadmeta.CollectorEvent {
	var annotations map[string]string
	if img.Spec == nil {
		annotations = nil
	} else {
		annotations = img.Spec.Annotations
	}

	var name string
	if len(img.RepoTags) > 0 {
		name = img.RepoTags[0]
	}

	os, arch, variant, labels, layers := parseImageInfo(info, crio.OverlayImagePath, img.Id)

	imgMeta := workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   img.Id,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		SizeBytes:    int64(img.Size_),
		RepoTags:     img.RepoTags,
		RepoDigests:  img.RepoDigests,
		SBOM:         sbom,
		OS:           os,
		Architecture: arch,
		Variant:      variant,
		Layers:       layers,
	}

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceRuntime,
		Entity: &imgMeta,
	}
}

// updateSBOMForImage updates the SBOM of an existing image if it's missing or outdated.
func (c *collector) updateSBOMForImage(imageID string, sbom *workloadmeta.SBOM) error {
	existingImg, err := c.store.GetImage(imageID)
	if err != nil || existingImg == nil {
		return fmt.Errorf("image %s not found in store for SBOM update", imageID)
	}

	if existingImg.SBOM != nil && existingImg.SBOM.Status == workloadmeta.Success {
		log.Debugf("SBOM for image ID %s is already up-to-date, skipping update", imageID)
		return nil
	}

	existingImg.SBOM = sbom
	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceRuntime,
			Entity: existingImg,
		},
	})
	return nil
}

// generateUnsetImageEvent generates an unset CollectorEvent for a removed or deleted image.
// This event can be used to notify that an image is no longer available.
func generateUnsetImageEvent(seenID workloadmeta.EntityID) workloadmeta.CollectorEvent {
	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.ContainerImageMetadata{
			EntityID: seenID,
		},
	}
}

// parseImageInfo extracts operating system, architecture, variant, labels, and layer history from image info metadata.
func parseImageInfo(info map[string]string, layerFilePath string, imgID string) (string, string, string, map[string]string, []workloadmeta.ContainerImageLayer) {
	var os, arch, variant string
	var layers []workloadmeta.ContainerImageLayer
	var labels map[string]string

	// Fetch additional layer information from the file
	layerDetails, err := parseLayerInfo(layerFilePath, imgID)
	if err != nil {
		log.Warnf("Failed to get layer mediaType and size: %v", err)
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

// parseLayerInfo reads a JSON file from the given path and returns a list of LayerInfo
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

// parsedInfo holds metadata extracted from image JSON, including labels and image spec details.
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
