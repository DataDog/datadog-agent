// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	log "github.com/cihub/seelog"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/content"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/gogo/protobuf/proto"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const imageTopicPrefix = "/images/"

func isImageTopic(topic string) bool {
	return strings.HasPrefix(topic, imageTopicPrefix)
}

func (c *collector) handleImageEvent(ctx context.Context, containerdEvent *containerdevents.Envelope) error {
	switch containerdEvent.Topic {
	case imageCreationTopic:
		event := &events.ImageCreate{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, event.Name)

	case imageUpdateTopic:
		event := &events.ImageUpdate{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, event.Name)

	case imageDeletionTopic:
		event := &events.ImageDelete{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		imageID, found := c.knownImages[event.Name]
		if !found {
			// Not necessarily an error. If an image had multiple names, it
			// could have already been deleted using other name
			return nil
		}

		c.store.Notify([]workloadmeta.CollectorEvent{
			{
				Type:   workloadmeta.EventTypeUnset,
				Source: workloadmeta.SourceRuntime,
				Entity: &workloadmeta.ContainerImageMetadata{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainerImageMetadata,
						ID:   imageID,
					},
				},
			},
		})

		delete(c.knownImages, event.Name)

		return nil
	default:
		return fmt.Errorf("unknown containerd image event topic %s, ignoring", containerdEvent.Topic)
	}
}

func (c *collector) handleImageCreateOrUpdate(ctx context.Context, namespace string, imageName string) error {
	img, err := c.containerdClient.Image(namespace, imageName)
	if err != nil {
		return fmt.Errorf("error getting image: %w", err)
	}

	return c.notifyEventForImage(ctx, namespace, img)
}

func (c *collector) notifyEventForImage(ctx context.Context, namespace string, img containerd.Image) error {
	ctxWithNamespace := namespaces.WithNamespace(ctx, namespace)

	manifest, err := images.Manifest(ctxWithNamespace, img.ContentStore(), img.Target(), img.Platform())
	if err != nil {
		return fmt.Errorf("error getting image manifest: %w", err)
	}

	layers, err := getLayersWithHistory(ctxWithNamespace, img.ContentStore(), manifest)
	if err != nil {
		_ = log.Warnf("error while getting layers with history: %s", err)

		// Not sure if the layers and history are always available. Instead of
		// returning an error, collect the image without this information.
	}

	platforms, err := images.Platforms(ctxWithNamespace, img.ContentStore(), manifest.Config)
	if err != nil {
		return fmt.Errorf("error getting image platforms: %w", err)
	}

	imageName := img.Name()
	imageID := manifest.Config.Digest.String()

	// We can get "create" events for images that already exist. That happens
	// when the same image is referenced with different names. For example,
	// datadog/agent:latest and datadog/agent:7 might refer to the same image.
	// Also, in some environments (at least with Kind), pulling an image like
	// datadog/agent:latest creates several events: in one of them the image
	// name is a digest, in other is something with the same format as
	// datadog/agent:7, and sometimes there's a temporary name prefixed with
	// "import-".
	existingImg, err := c.store.GetImage(imageID)
	if err == nil {
		updatedImg := existingImg.DeepCopy().(*workloadmeta.ContainerImageMetadata) // Avoid race conditions
		changed := c.updateContainerImageMetadata(updatedImg, imageName)

		if changed {
			c.store.Notify([]workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceRuntime,
					Entity: updatedImg,
				},
			})

			c.updateKnownImages(imageName, imageID)
		}

		return nil
	}

	shortName := ""
	parsedImg, err := workloadmeta.NewContainerImage(imageName)
	if err != nil {
		_ = log.Warn("Can't get image short name")
	} else {
		shortName = parsedImg.ShortName
	}

	var repoDigests []string
	var repoTags []string
	if strings.Contains(imageName, "@sha256:") {
		repoDigests = append(repoDigests, imageName)
	} else {
		repoDigests = append(repoDigests, imageName+"@"+img.Target().Digest.String())
		repoTags = append(repoTags, imageName)
	}

	var totalSizeBytes int64 = 0
	for _, layer := range manifest.Layers {
		totalSizeBytes += layer.Size
	}

	var os, osVersion, architecture, variant string
	// If there are multiple platforms, return the info about the first one
	if len(platforms) >= 1 {
		os = platforms[0].OS
		osVersion = platforms[0].OSVersion
		architecture = platforms[0].Architecture
		variant = platforms[0].Variant
	}

	workloadmetaImg := workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imageID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      imageName,
			Namespace: namespace,
			Labels:    img.Labels(),
		},
		ShortName:    shortName,
		RepoTags:     repoTags,
		RepoDigests:  repoDigests,
		MediaType:    manifest.MediaType,
		SizeBytes:    totalSizeBytes,
		OS:           os,
		OSVersion:    osVersion,
		Architecture: architecture,
		Variant:      variant,
		Layers:       layers,
	}

	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceRuntime,
			Entity: &workloadmetaImg,
		},
	})

	c.updateKnownImages(imageName, imageID)

	return nil
}

func (c *collector) updateKnownImages(imageName string, newImageID string) {
	currentImageID, found := c.knownImages[imageName]

	// If the image name is already pointing to an ID, we need to delete it from
	// the repo tags of the image with that ID.
	if found {
		existingImg, err := c.store.GetImage(currentImageID)
		if err == nil {
			existingImgCopy := existingImg.DeepCopy().(*workloadmeta.ContainerImageMetadata) // Avoid race conditions
			for i, repoTag := range existingImgCopy.RepoTags {
				if repoTag == imageName {
					existingImgCopy.RepoTags = append(existingImgCopy.RepoTags[:i], existingImgCopy.RepoTags[i+1:]...)

					c.store.Notify([]workloadmeta.CollectorEvent{
						{
							Type:   workloadmeta.EventTypeSet,
							Source: workloadmeta.SourceRuntime,
							Entity: existingImgCopy,
						},
					})

					break
				}
			}
		}
	}

	c.knownImages[imageName] = newImageID
}

func getLayersWithHistory(ctx context.Context, store content.Store, manifest ocispec.Manifest) ([]workloadmeta.ContainerImageLayer, error) {
	blob, err := content.ReadBlob(ctx, store, manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("error while getting image contents: %w", err)
	}

	var ocispecImage ocispec.Image
	if err = json.Unmarshal(blob, &ocispecImage); err != nil {
		return nil, fmt.Errorf("error while unmarshaling image: %w", err)
	}

	var layers []workloadmeta.ContainerImageLayer

	// The layers in the manifest don't include the history, and the only way to
	// match the history with each layer is to rely on the order and take into
	// account that some history objects don't have an associated layer
	// (emptyLayer = true).

	history := ocispecImage.History
	manifestLayersIdx := 0

	for _, historyPoint := range history {
		layer := workloadmeta.ContainerImageLayer{
			History: historyPoint,
		}

		if !historyPoint.EmptyLayer {
			if manifestLayersIdx >= len(manifest.Layers) {
				// This should never happen. len(manifest.Layers) should equal
				// len(history) minus the number of history points with
				// emptyLayer = false.
				return nil, fmt.Errorf("error while extracting image layer history")
			}

			manifestLayer := manifest.Layers[manifestLayersIdx]
			manifestLayersIdx += 1

			layer.MediaType = manifestLayer.MediaType
			layer.Digest = manifestLayer.Digest.String()
			layer.SizeBytes = manifestLayer.Size
			layer.URLs = manifestLayer.URLs
		}

		layers = append(layers, layer)
	}

	if manifestLayersIdx != len(manifest.Layers) {
		// This should never happen. Same case as above.
		return nil, fmt.Errorf("error while extracting image layer history")
	}

	return layers, nil
}

// updateContainerImageMetadata Updates the given container image metadata so
// that it takes into account the new image name that refers to it. Returns a
// boolean that indicates if the image metadata was updated.
// There are 2 possible changes to be made:
//  1. Some environments refer to the same image with multiple names. When
//     that's the case, give precedence to the name with repo and tag instead of
//     the name that includes a digest. This is just to show names that are more
//     user-friendly (the digests are already present in other attributes like ID,
//     and repo digest).
//  2. Add the new name to the repo tags of the image. Notice that repo names
//     are not digests.
func (c *collector) updateContainerImageMetadata(imageMetadata *workloadmeta.ContainerImageMetadata, newName string) bool {
	if strings.Contains(newName, "sha256:") {
		// Nothing to do. It's not going to replace the current name, and it's
		// not a repo tag.
		return false
	}

	changed := false

	// If the current name is a digest and the new one is not, replace.
	if strings.Contains(imageMetadata.Name, "sha256:") {
		imageMetadata.Name = newName

		parsedName, err := workloadmeta.NewContainerImage(newName)
		if err == nil {
			imageMetadata.ShortName = parsedName.ShortName
		}

		changed = true
	}

	// Add new repo tag if not already present.
	addNewRepoTag := true
	for _, existingRepoTag := range imageMetadata.RepoTags {
		if existingRepoTag == newName {
			addNewRepoTag = false
			break
		}
	}
	if addNewRepoTag {
		imageMetadata.RepoTags = append(imageMetadata.RepoTags, newName)
		changed = true
	}

	return changed
}
