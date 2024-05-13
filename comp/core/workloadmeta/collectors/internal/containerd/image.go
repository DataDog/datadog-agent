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

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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

// getPreferredName will return a user-friendly image name if it exists, otherwise
// for example the name not including the digest.
func (images *knownImages) getPreferredName(imageID string) string {
	var res = ""
	for ref := range images.namesByID[imageID] {
		if res == "" && isAnImageID(ref) {
			res = ref
		} else if isARepoDigest(ref) {
			res = ref // Prefer the repo digest
			break
		} else {
			res = ref // Then repo tag
		}
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

// pullImageReferences pulls all references from containerd for a given DIGEST
// Note: the DIGEST here is the same as digest (repo digest) field returned from "ctr -n NAMESPACE ls"
// rather than config.digest (imageID), which is the digest of the image config blob.
// In general, 3 reference names are returned for a given DIGEST: repo tag, repo digest, and imageID.
func (c *collector) pullImageReferences(namespace string, img containerd.Image) []string {
	var refs []string
	digest := img.Target().Digest.String()
	if !strings.HasPrefix(digest, "sha256") {
		return refs // not a valid digest
	}

	// Get all references for the imageID
	referenceImages, err := c.containerdClient.ListImagesWithDigest(namespace, digest)
	if err == nil {
		for _, image := range referenceImages {
			imageName := image.Name()
			refs = append(refs, imageName)
		}
	} else {
		log.Debugf("failed to get reference images for image: %s, repo digests will be missing: %v", img.Name(), err)
	}
	return refs
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

// createOrUpdateImageMetadata: Create image metadata from containerd image and manifest if not already present
// Update image metadata by adding references when existing entity is found
// return nil when it fails to get image manifest
func (c *collector) createOrUpdateImageMetadata(ctx context.Context,
	namespace string,
	img containerd.Image,
	sbom *workloadmeta.SBOM) (*workloadmeta.ContainerImageMetadata, error) {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()

	ctxWithNamespace := namespaces.WithNamespace(ctx, namespace)

	// Build initial workloadmeta.ContainerImageMetadata from manifest and image
	manifest, err := images.Manifest(ctxWithNamespace, img.ContentStore(), img.Target(), img.Platform())
	if err != nil {
		return nil, fmt.Errorf("error getting image manifest: %w", err)
	}

	totalSizeBytes := manifest.Config.Size
	for _, layer := range manifest.Layers {
		totalSizeBytes += layer.Size
	}

	wlmImage := workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   manifest.Config.Digest.String(),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      img.Name(),
			Namespace: namespace,
		},
		MediaType: manifest.MediaType,
		SBOM:      sbom,
		SizeBytes: totalSizeBytes,
	}
	// Only pull all image references if not already present
	if _, found := c.knownImages.getImageID(wlmImage.Name); !found {
		references := c.pullImageReferences(namespace, img)
		for _, ref := range references {
			c.knownImages.addReference(ref, wlmImage.ID)
		}
	}
	// update knownImages with current reference name
	c.knownImages.addReference(wlmImage.Name, wlmImage.ID)

	// Fill image based on manifest and config, we are not failing if this step fails
	// as we can live without layers or labels
	if err := extractFromConfigBlob(ctxWithNamespace, img, manifest, &wlmImage); err != nil {
		log.Infof("failed to get image config for image: %s, layers and labels will be missing: %v", img.Name(), err)
	}

	wlmImage.RepoTags = c.knownImages.getRepoTags(wlmImage.ID)
	wlmImage.RepoDigests = c.knownImages.getRepoDigests(wlmImage.ID)

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
	wlmImage.Name = c.knownImages.getPreferredName(wlmImage.ID)
	existingImg, err := c.store.GetImage(wlmImage.ID)
	if err == nil {
		if strings.Contains(wlmImage.Name, "sha256:") && !strings.Contains(existingImg.Name, "sha256:") {
			wlmImage.Name = existingImg.Name
		}
	}

	if wlmImage.SBOM == nil {
		wlmImage.SBOM = &workloadmeta.SBOM{
			Status: workloadmeta.Pending,
		}
	}

	// The CycloneDX should contain the RepoTags and RepoDigests but the scanner might
	// not be able to inject them. For example, if we use the scanner from filesystem or
	// if the `imgMeta` object does not contain all the metadata when it is sent.
	// We add them here to make sure they are present.
	wlmImage.SBOM = util.UpdateSBOMRepoMetadata(wlmImage.SBOM, wlmImage.RepoTags, wlmImage.RepoDigests)
	return &wlmImage, nil
}

func (c *collector) notifyEventForImage(ctx context.Context, namespace string, img containerd.Image, sbom *workloadmeta.SBOM) error {
	wlmImage, err := c.createOrUpdateImageMetadata(ctx, namespace, img, sbom)
	if err != nil {
		return err
	}
	c.store.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceRuntime,
			Entity: wlmImage,
		},
	})
	return nil
}

func extractFromConfigBlob(ctx context.Context, img containerd.Image, manifest ocispec.Manifest, outImage *workloadmeta.ContainerImageMetadata) error {
	// First extract platform from Config descriptor
	extractPlatform(manifest.Config.Platform, outImage)

	imageConfigBlob, err := content.ReadBlob(ctx, img.ContentStore(), manifest.Config)
	if err != nil {
		return fmt.Errorf("error getting image config: %w", err)
	}

	var ocispecImage ocispec.Image
	if err = json.Unmarshal(imageConfigBlob, &ocispecImage); err != nil {
		return fmt.Errorf("error while unmarshaling image config: %w", err)
	}

	// If we are able to read config, override with values from config if any
	extractPlatform(&ocispecImage.Platform, outImage)

	outImage.Layers = getLayersWithHistory(ocispecImage, manifest)
	outImage.Labels = getImageLabels(img, ocispecImage)
	return nil
}

func extractPlatform(platform *ocispec.Platform, outImage *workloadmeta.ContainerImageMetadata) {
	if platform == nil {
		return
	}

	if platform.Architecture != "" {
		outImage.Architecture = platform.Architecture
	}

	if platform.OS != "" {
		outImage.OS = platform.OS
	}

	if platform.OSVersion != "" {
		outImage.OSVersion = platform.OSVersion
	}

	if platform.Variant != "" {
		outImage.Variant = platform.Variant
	}
}

func getLayersWithHistory(ocispecImage ocispec.Image, manifest ocispec.Manifest) []workloadmeta.ContainerImageLayer {
	var layers []workloadmeta.ContainerImageLayer

	// The layers in the manifest don't include the history, and the only way to
	// match the history with each layer is to rely on the order and take into
	// account that some history objects don't have an associated layer
	// (emptyLayer = true).
	// History is optional in OCI Spec, so we have no guarantee to be able to get it.

	historyIndex := 0
	for _, manifestLayer := range manifest.Layers {
		// Look for next history point with emptyLayer = false
		historyFound := false
		for ; historyIndex < len(ocispecImage.History); historyIndex++ {
			if !ocispecImage.History[historyIndex].EmptyLayer {
				historyFound = true
				break
			}
		}

		layer := workloadmeta.ContainerImageLayer{
			MediaType: manifestLayer.MediaType,
			Digest:    manifestLayer.Digest.String(),
			SizeBytes: manifestLayer.Size,
			URLs:      manifestLayer.URLs,
		}
		if historyFound {
			layer.History = &ocispecImage.History[historyIndex]
			historyIndex++
		}

		layers = append(layers, layer)
	}

	return layers
}

func getImageLabels(img containerd.Image, ocispecImage ocispec.Image) map[string]string {
	// Labels() does not return the labels set in the Dockerfile. They are in
	// the config descriptor.
	// When running on Kubernetes Labels() only returns io.cri-containerd
	// labels.
	labels := map[string]string{}

	for labelName, labelValue := range img.Labels() {
		labels[labelName] = labelValue
	}

	for labelName, labelValue := range ocispecImage.Config.Labels {
		labels[labelName] = labelValue
	}

	return labels
}
