// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/content"
	containerdevents "github.com/containerd/containerd/events"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"google.golang.org/protobuf/proto"
)

const imageTopicPrefix = "/images/"

// We cannot get all the information that we need from a single call to
// containerd. This type stores the information that we need to know about the
// images that we have already processed.
//
// Things to take into account:
//
// - Events from containerd only include an image name, and it does not always
// correspond to an image ID. When a delete event arrives it'll contain a name,
// but we won't be able to access the ID because the image is already gone,
// that's why we need to keep the IDs => names relationships.
//
// - An image ID can be referenced by multiple names.
//
// - A name can have multiple formats:
//   - image ID: starts with "sha256:"
//   - repo digest. They contain "@sha256:". Example: gcr.io/datadoghq/agent@sha256:3a19076bfee70900a600b8e3ee2cc30d5101d1d3d2b33654f1a316e596eaa4e0
//   - repo tag. Example: gcr.io/datadoghq/agent:7
type knownImages struct {
	// Store IDs and names in both directions for efficient access.
	idsByName       map[string]string              // map name => ID
	namesByID       map[string]map[string]struct{} // map ID => set of names
	repoTagsByID    map[string]map[string]struct{} // map ID => set of repo tags
	repoDigestsByID map[string]map[string]struct{} // map ID => set of repo digests
}

func newKnownImages() *knownImages {
	return &knownImages{
		idsByName:       make(map[string]string),
		namesByID:       make(map[string]map[string]struct{}),
		repoTagsByID:    make(map[string]map[string]struct{}),
		repoDigestsByID: make(map[string]map[string]struct{}),
	}
}

func (images *knownImages) addReference(imageName string, imageID string) {
	previousIDReferenced, found := images.idsByName[imageName]
	if found && previousIDReferenced != imageID {
		images.deleteReference(imageName, previousIDReferenced)
	}

	images.idsByName[imageName] = imageID

	if images.namesByID[imageID] == nil {
		images.namesByID[imageID] = make(map[string]struct{})
	}
	images.namesByID[imageID][imageName] = struct{}{}

	if isAnImageID(imageName) {
		return
	}

	if isARepoDigest(imageName) {
		if images.repoDigestsByID[imageID] == nil {
			images.repoDigestsByID[imageID] = make(map[string]struct{})
		}
		images.repoDigestsByID[imageID][imageName] = struct{}{}
		return
	}

	// The name is not an image ID or a repo digest, so it has to be a repo tag
	if images.repoTagsByID[imageID] == nil {
		images.repoTagsByID[imageID] = make(map[string]struct{})
	}
	images.repoTagsByID[imageID][imageName] = struct{}{}
	return
}

func (images *knownImages) deleteReference(imageName string, imageID string) {
	delete(images.idsByName, imageName)

	if images.namesByID[imageID] != nil {
		delete(images.namesByID[imageID], imageName)
	}

	if isAnImageID(imageName) {
		return
	}

	if isARepoDigest(imageName) {
		if images.repoDigestsByID[imageID] == nil {
			return
		}
		delete(images.repoDigestsByID[imageID], imageName)
		if len(images.repoDigestsByID[imageID]) == 0 {
			delete(images.repoDigestsByID, imageID)
		}
		return
	}

	// The name is not an image ID or a repo digest, so it has to be a repo tag
	if images.repoTagsByID[imageID] == nil {
		return
	}
	delete(images.repoTagsByID[imageID], imageName)
	if len(images.repoTagsByID[imageID]) == 0 {
		delete(images.repoTagsByID, imageID)
	}
}

func (images *knownImages) getImageID(imageName string) (string, bool) {
	id, found := images.idsByName[imageName]
	return id, found
}

func (images *knownImages) getRepoTags(imageID string) []string {
	var res []string
	for repoTag := range images.repoTagsByID[imageID] {
		res = append(res, repoTag)
	}
	return res
}

func (images *knownImages) getRepoDigests(imageID string) []string {
	var res []string
	for repoDigest := range images.repoDigestsByID[imageID] {
		res = append(res, repoDigest)
	}
	return res
}

// returns any of the existing references for the imageID. Returns empty if the
// ID is not referenced.
func (images *knownImages) getAReference(imageID string) string {
	for ref := range images.namesByID[imageID] {
		return ref
	}

	return ""
}

func isImageTopic(topic string) bool {
	return strings.HasPrefix(topic, imageTopicPrefix)
}

func isAnImageID(imageName string) bool {
	return strings.HasPrefix(imageName, "sha256")
}

func isARepoDigest(imageName string) bool {
	return strings.Contains(imageName, "@sha256:")
}

func (c *collector) handleImageEvent(ctx context.Context, containerdEvent *containerdevents.Envelope) error {
	switch containerdEvent.Topic {
	case imageCreationTopic:
		event := &events.ImageCreate{}
		if err := proto.Unmarshal(containerdEvent.Event.GetValue(), event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, event.Name, nil)

	case imageUpdateTopic:
		event := &events.ImageUpdate{}
		if err := proto.Unmarshal(containerdEvent.Event.GetValue(), event); err != nil {
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, event.Name, nil)

	case imageDeletionTopic:
		c.handleImagesMut.Lock()

		event := &events.ImageDelete{}
		if err := proto.Unmarshal(containerdEvent.Event.GetValue(), event); err != nil {
			c.handleImagesMut.Unlock()
			return fmt.Errorf("error unmarshaling containerd event: %w", err)
		}

		imageID, found := c.knownImages.getImageID(event.Name)
		if !found {
			c.handleImagesMut.Unlock()
			return nil
		}

		c.knownImages.deleteReference(event.Name, imageID)

		if ref := c.knownImages.getAReference(imageID); ref != "" {
			// Image is still referenced by a different name, so don't delete
			// the image, but we need to update its repo tags and digest tags.
			// Updating workloadmeta entities directly is not thread-safe,
			// that's why we generate an update event here.
			c.handleImagesMut.Unlock()
			return c.handleImageCreateOrUpdate(ctx, containerdEvent.Namespace, ref, nil)
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

		c.handleImagesMut.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown containerd image event topic %s, ignoring", containerdEvent.Topic)
	}
}

func (c *collector) handleImageCreateOrUpdate(ctx context.Context, namespace string, imageName string, bom *workloadmeta.SBOM) error {
	img, err := c.containerdClient.Image(namespace, imageName)
	if err != nil {
		return fmt.Errorf("error getting image: %w", err)
	}

	return c.notifyEventForImage(ctx, namespace, img, bom)
}

func (c *collector) notifyEventForImage(ctx context.Context, namespace string, img containerd.Image, bom *workloadmeta.SBOM) error {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()

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
	imageID := manifest.Config.Digest.String()

	c.knownImages.addReference(imageName, imageID)

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
		}

		if existingBOM == nil && existingImg.SBOM != nil {
			existingBOM = existingImg.SBOM
		}
	}

	totalSizeBytes := manifest.Config.Size
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
		RepoTags:     c.knownImages.getRepoTags(imageID),
		RepoDigests:  c.knownImages.getRepoDigests(imageID),
		MediaType:    manifest.MediaType,
		SizeBytes:    totalSizeBytes,
		OS:           os,
		OSVersion:    osVersion,
		Architecture: architecture,
		Variant:      variant,
		Layers:       layers,
		SBOM:         existingBOM,
	}

	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceRuntime,
			Entity: &workloadmetaImg,
		},
	})

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
			manifestLayersIdx++

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
