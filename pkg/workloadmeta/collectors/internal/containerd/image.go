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
	"sync"

	"github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

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

// Events from containerd contain an image name, but not IDs. When a delete
// event arrives it'll contain a name, but we won't be able to access the ID
// because the image is already gone, that's why we need to keep the IDs =>
// names relationships.
type knownImages struct {
	// Needed because this is accessed by the goroutine handling events and also the one that extracts SBOMs
	mut sync.Mutex

	// Store IDs and names in both directions for efficient access
	idsByName map[string]string              // map name => ID
	namesByID map[string]map[string]struct{} // map ID => set of names
}

func newKnownImages() *knownImages {
	return &knownImages{
		idsByName: make(map[string]string),
		namesByID: make(map[string]map[string]struct{}),
	}
}

func (images *knownImages) addAssociation(imageName string, imageID string) {
	images.mut.Lock()
	defer images.mut.Unlock()

	images.idsByName[imageName] = imageID

	if images.namesByID[imageID] == nil {
		images.namesByID[imageID] = make(map[string]struct{})
	}
	images.namesByID[imageID][imageName] = struct{}{}
}

func (images *knownImages) deleteAssociation(imageName string, imageID string) {
	images.mut.Lock()
	defer images.mut.Unlock()

	delete(images.idsByName, imageName)

	if images.namesByID[imageID] == nil {
		return
	}

	delete(images.namesByID[imageID], imageName)
	if len(images.namesByID[imageID]) == 0 {
		delete(images.namesByID, imageID)
	}
}

func (images *knownImages) getImageID(imageName string) (string, bool) {
	images.mut.Lock()
	defer images.mut.Unlock()

	id, found := images.idsByName[imageName]
	return id, found
}

func (images *knownImages) isReferenced(imageID string) bool {
	images.mut.Lock()
	defer images.mut.Unlock()

	return len(images.namesByID[imageID]) > 0
}

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

		return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, event.Name, nil)

	case imageUpdateTopic:
		event := &events.ImageUpdate{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, event.Name, nil)

	case imageDeletionTopic:
		event := &events.ImageDelete{}
		if err := proto.Unmarshal(containerdEvent.Event.Value, event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		imageID, found := c.knownImages.getImageID(event.Name)
		if !found {
			return nil
		}

		c.knownImages.deleteAssociation(event.Name, imageID)

		if c.knownImages.isReferenced(imageID) {
			// Image is still referenced by a different name. Don't delete the
			// image, but update its repo tags.
			return c.deleteRepoTagOfImage(ctx, containerdEvent.Namespace, imageID, event.Name)
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

		return nil
	default:
		return fmt.Errorf("unknown containerd image event topic %s, ignoring", containerdEvent.Topic)
	}
}

func (c *collector) handleImageCreateOrUpdate(ctx context.Context, namespace string, imageName string, bom *cyclonedx.BOM) error {
	img, err := c.containerdClient.Image(namespace, imageName)
	if err != nil {
		return fmt.Errorf("error getting image: %w", err)
	}

	return c.notifyEventForImage(ctx, namespace, img, bom)
}

func (c *collector) notifyEventForImage(ctx context.Context, namespace string, img containerd.Image, bom *cyclonedx.BOM) error {
	ctxWithNamespace := namespaces.WithNamespace(ctx, namespace)

	manifest, err := images.Manifest(ctxWithNamespace, img.ContentStore(), img.Target(), img.Platform())
	if err != nil {
		return fmt.Errorf("error getting image manifest: %w", err)
	}

	layers, err := getLayersWithHistory(ctxWithNamespace, img.ContentStore(), manifest)
	if err != nil {
		log.Warnf("error while getting layers with history: %s", err)

		// Not sure if the layers and history are always available. Instead of
		// returning an error, collect the image without this information.
	}

	platforms, err := images.Platforms(ctxWithNamespace, img.ContentStore(), manifest.Config)
	if err != nil {
		return fmt.Errorf("error getting image platforms: %w", err)
	}

	imageName := img.Name()
	registry := ""
	shortName := ""
	parsedImg, err := workloadmeta.NewContainerImage(imageName)
	if err == nil {
		// Don't set a short name. We know that some images handled here contain
		// "sha256" in the name, and those don't have a short name.
	} else {
		registry = parsedImg.Registry
		shortName = parsedImg.ShortName
	}

	imageID := manifest.Config.Digest.String()

	existingBOM := bom

	// We can get "create" events for images that already exist. That happens
	// when the same image is referenced with different names. For example,
	// datadog/agent:latest and datadog/agent:7 might refer to the same image.
	// Also, in some environments (at least with Kind), pulling an image like
	// datadog/agent:latest creates several events: in one of them the image
	// name is a digest, in other is something with the same format as
	// datadog/agent:7, and sometimes there's a temporary name prefixed with
	// "import-".
	// When that happens, give precedence to the name with repo and tag instead
	// of the name that includes a digest. This is just to show names that are
	// more user-friendly (the digests are already present in other attributes
	// like ID, and repo digest).
	existingImg, err := c.store.GetImage(imageID)
	if err == nil {
		if strings.Contains(imageName, "sha256:") && !strings.Contains(existingImg.Name, "sha256:") {
			imageName = existingImg.Name
			shortName = existingImg.ShortName
		}

		if existingBOM == nil && existingImg.CycloneDXBOM != nil {
			existingBOM = existingImg.CycloneDXBOM
		}
	}

	var repoDigests []string
	if strings.Contains(imageName, "@sha256:") {
		repoDigests = append(repoDigests, imageName)
	} else {
		repoDigests = append(repoDigests, imageName+"@"+img.Target().Digest.String())

		repoTagAlreadyPresent := false
		for _, repoTag := range c.repoTags[imageID] {
			if repoTag == imageName {
				repoTagAlreadyPresent = true
				break
			}
		}

		if !repoTagAlreadyPresent {
			c.repoTags[imageID] = append(c.repoTags[imageID], imageName)
		}
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
		Registry:     registry,
		ShortName:    shortName,
		RepoTags:     c.repoTags[imageID],
		RepoDigests:  repoDigests,
		MediaType:    manifest.MediaType,
		SizeBytes:    totalSizeBytes,
		OS:           os,
		OSVersion:    osVersion,
		Architecture: architecture,
		Variant:      variant,
		Layers:       layers,
		CycloneDXBOM: existingBOM,
	}

	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceRuntime,
			Entity: &workloadmetaImg,
		},
	})

	if existingBOM == nil && sbomCollectionIsEnabled() {
		// Notify image scanner
		c.imagesToScan <- namespacedImage{
			namespace: namespace,
			image:     img,
			imageID:   imageID,
		}
	}

	return c.updateKnownImages(ctx, namespace, imageName, imageID)
}

// Updates the map with the image name => image ID relationships and also the repo tags
func (c *collector) updateKnownImages(ctx context.Context, namespace string, imageName string, newImageID string) error {
	oldImageID, found := c.knownImages.getImageID(imageName)
	c.knownImages.addAssociation(imageName, newImageID)

	// If the image name is already pointing to an ID, we need to delete the name from
	// the repo tags of the image with that ID.
	if found && newImageID != oldImageID {
		c.knownImages.deleteAssociation(imageName, oldImageID)
		return c.deleteRepoTagOfImage(ctx, namespace, oldImageID, imageName)
	}

	return nil
}

func (c *collector) deleteRepoTagOfImage(ctx context.Context, namespace string, imageID string, repoTagToDelete string) error {
	repoTagDeleted := false

	for i, repoTag := range c.repoTags[imageID] {
		if repoTag == repoTagToDelete {
			c.repoTags[imageID] = append(c.repoTags[imageID][:i], c.repoTags[imageID][i+1:]...)
			repoTagDeleted = true
			break
		}
	}

	if repoTagDeleted && len(c.repoTags[imageID]) > 0 {
		// We need to notify to workloadmeta that the image has changed.
		// Updating workloadmeta entities directly is not thread-safe, that's
		// why we generate an update event here instead.
		if err := c.handleImageCreateOrUpdate(ctx, namespace, c.repoTags[imageID][len(c.repoTags[imageID])-1], nil); err != nil {
			return err
		}
	}

	return nil
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

func sbomCollectionIsEnabled() bool {
	return imageMetadataCollectionIsEnabled() && config.Datadog.GetBool("workloadmeta.image_metadata_collection.collect_sboms")
}
