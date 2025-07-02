// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && containerd

package trivy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/errdefs"
	refdocker "github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	dimage "github.com/docker/docker/api/types/image"
	dclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerdCollector defines the conttainerd collector name
const ContainerdCollector = "containerd"

// Code ported from https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/containerd.go
type familiarNamed string

func (n familiarNamed) Name() string {
	return strings.Split(string(n), ":")[0]
}

func (n familiarNamed) Tag() string {
	s := strings.Split(string(n), ":")
	if len(s) < 2 {
		return ""
	}

	return s[1]
}

func (n familiarNamed) String() string {
	return string(n)
}

// Code ported from https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/containerd.go
func imageWriter(client *containerd.Client, img containerd.Image) imageSave {
	return func(ctx context.Context, ref []string, _ ...dclient.ImageSaveOption) (io.ReadCloser, error) {
		if len(ref) < 1 {
			return nil, errors.New("no image reference")
		}
		imgOpts := archive.WithImage(client.ImageService(), ref[0])
		manifestOpts := archive.WithManifest(img.Target())
		platOpts := archive.WithPlatform(img.Platform())
		pr, pw := io.Pipe()
		go func() {
			pw.CloseWithError(archive.Export(ctx, client.ContentStore(), pw, imgOpts, manifestOpts, platOpts))
		}()
		return pr, nil
	}
}

// Custom code based on https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/containerd.go `ContainerdImage`
func convertContainerdImage(ctx context.Context, client *containerd.Client, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (*image, func(), error) {
	ctx = namespaces.WithNamespace(ctx, imgMeta.Namespace)
	cleanup := func() {}

	f, err := os.CreateTemp("", "fanal-containerd-*")
	if err != nil {
		return nil, cleanup, fmt.Errorf("failed to create a temporary file: %w", err)
	}

	cleanup = func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}

	insp, history, ref, err := inspect(ctx, imgMeta, img)
	if err != nil {
		return nil, cleanup, fmt.Errorf("inspect error: %w", err) // Note: the original code doesn't return "cleanup".
	}

	return &image{
		name:    img.Name(),
		opener:  imageOpener(ctx, ContainerdCollector, ref.String(), f, imageWriter(client, img)),
		inspect: insp,
		history: history,
	}, cleanup, nil
}

// readImageConfig reads the config spec (`application/vnd.oci.image.config.v1+json`) for img.platform from content store.
// ported from https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/containerd.go
func readImageConfig(ctx context.Context, img containerd.Image) (ocispec.Image, ocispec.Descriptor, error) {
	var config ocispec.Image

	configDesc, err := img.Config(ctx) // aware of img.platform
	if err != nil {
		return config, configDesc, err
	}
	p, err := content.ReadBlob(ctx, img.ContentStore(), configDesc)
	if err != nil {
		return config, configDesc, err
	}
	if err = json.Unmarshal(p, &config); err != nil {
		return config, configDesc, err
	}
	return config, configDesc, nil
}

// ported from https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/containerd.go
func inspect(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (dimage.InspectResponse, []v1.History, refdocker.Reference, error) {
	ref := familiarNamed(img.Name())

	imgConfig, imgConfigDesc, err := readImageConfig(ctx, img)
	if err != nil {
		return dimage.InspectResponse{}, nil, nil, err
	}

	var lastHistory ocispec.History
	if len(imgConfig.History) > 0 {
		lastHistory = imgConfig.History[len(imgConfig.History)-1]
	}

	var history []v1.History
	for _, h := range imgConfig.History {
		var created time.Time
		if h.Created != nil {
			created = *h.Created
		}

		history = append(history, v1.History{
			Author:     h.Author,
			Created:    v1.Time{Time: created},
			CreatedBy:  h.CreatedBy,
			Comment:    h.Comment,
			EmptyLayer: h.EmptyLayer,
		})
	}

	portSet := make(nat.PortSet)
	for k := range imgConfig.Config.ExposedPorts {
		portSet[nat.Port(k)] = struct{}{}
	}
	created := ""
	if lastHistory.Created != nil {
		created = lastHistory.Created.Format(time.RFC3339Nano)
	}

	return dimage.InspectResponse{
		ID:          imgConfigDesc.Digest.String(),
		RepoTags:    imgMeta.RepoTags,
		RepoDigests: imgMeta.RepoDigests,
		Comment:     lastHistory.Comment,
		Created:     created,
		Author:      lastHistory.Author,
		Config: &container.Config{
			User:         imgConfig.Config.User,
			ExposedPorts: portSet,
			Env:          imgConfig.Config.Env,
			Cmd:          imgConfig.Config.Cmd,
			Volumes:      imgConfig.Config.Volumes,
			WorkingDir:   imgConfig.Config.WorkingDir,
			Entrypoint:   imgConfig.Config.Entrypoint,
			Labels:       imgConfig.Config.Labels,
		},
		Architecture: imgConfig.Architecture,
		Os:           imgConfig.OS,
		RootFS: dimage.RootFS{
			Type: imgConfig.RootFS.Type,
			Layers: lo.Map(imgConfig.RootFS.DiffIDs, func(d digest.Digest, _ int) string {
				return d.String()
			}),
		},
	}, history, ref, nil
}

const (
	cleanupTimeout = 30 * time.Second
)

type fakeContainerdContainer struct {
	*fakeContainer
	*image
}

func (c *fakeContainerdContainer) LayerByDiffID(hash string) (ftypes.LayerPath, error) {
	return c.fakeContainer.LayerByDiffID(hash)
}

func (c *fakeContainerdContainer) LayerByDigest(hash string) (ftypes.LayerPath, error) {
	return c.fakeContainer.LayerByDigest(hash)
}

func (c *fakeContainerdContainer) Layers() (layers []ftypes.LayerPath) {
	return c.fakeContainer.Layers()
}

// ContainerdAccessor is a function that should return a containerd client
type ContainerdAccessor func() (cutil.ContainerdItf, error)

// ScanContainerdImageFromSnapshotter scans containerd image directly from the snapshotter
func (c *Collector) ScanContainerdImageFromSnapshotter(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertContainerdImage(ctx, client.RawClient(), imgMeta, img)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}

	// Computing duration of containerd lease
	deadline, _ := ctx.Deadline()
	expiration := deadline.Sub(time.Now().Add(cleanupTimeout))
	clClient := client.RawClient()
	imageID := imgMeta.ID

	mounts, err := client.Mounts(ctx, expiration, imgMeta.Namespace, img)
	if err != nil {
		return nil, fmt.Errorf("unable to get mounts for image %s, err: %w", imgMeta.ID, err)
	}

	layers := extractLayersFromOverlayFSMounts(mounts)
	if len(layers) == 0 {
		return nil, fmt.Errorf("unable to extract layers from overlayfs mounts %+v for image %s", mounts, imgMeta.ID)
	}

	ctx = namespaces.WithNamespace(ctx, imgMeta.Namespace)
	// Adding a lease to cleanup dandling snaphots at expiration
	ctx, done, err := clClient.WithLease(ctx,
		leases.WithID(imageID),
		leases.WithExpiration(expiration),
		leases.WithLabels(map[string]string{
			"containerd.io/gc.ref.snapshot." + containerd.DefaultSnapshotter: imageID,
		}),
	)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return nil, fmt.Errorf("unable to get a lease, err: %w", err)
	}

	fakeContainer, err := newFakeContainer(layers, imgMeta, fanalImage.inspect.RootFS.Layers)
	if err != nil {
		return nil, err
	}

	report, err := c.scanOverlayFS(ctx, layers, &fakeContainerdContainer{
		image:         fanalImage,
		fakeContainer: fakeContainer,
	}, imgMeta, scanOptions)

	if err := done(ctx); err != nil {
		log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
	}

	return report, err
}

// ScanContainerdImage scans containerd image by exporting it and scanning the tarball
func (c *Collector) ScanContainerdImage(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	fanalImage, cleanup, err := convertContainerdImage(ctx, client.RawClient(), imgMeta, img)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, fmt.Errorf("unable to convert containerd image, err: %w", err)
	}

	return c.scanImage(ctx, fanalImage, imgMeta, scanOptions)
}

// ScanContainerdImageFromFilesystem scans containerd image from file-system
func (c *Collector) ScanContainerdImageFromFilesystem(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image, client cutil.ContainerdItf, scanOptions sbom.ScanOptions) (sbom.Report, error) {
	imagePath, err := os.MkdirTemp("", "containerd-image-*")
	if err != nil {
		return nil, fmt.Errorf("unable to create temp dir, err: %w", err)
	}
	defer func() {
		err := os.RemoveAll(imagePath)
		if err != nil {
			log.Errorf("Unable to remove temp dir: %s, err: %v", imagePath, err)
		}
	}()

	// Computing duration of containerd lease
	deadline, _ := ctx.Deadline()
	expiration := deadline.Sub(time.Now().Add(cleanupTimeout))

	cleanUp, err := client.MountImage(ctx, expiration, imgMeta.Namespace, img, imagePath)
	if err != nil {
		return nil, fmt.Errorf("unable to mount containerd image, err: %w", err)
	}

	defer func() {
		cleanUpContext, cleanUpContextCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		err := cleanUp(cleanUpContext)
		cleanUpContextCancel()
		if err != nil {
			log.Errorf("Unable to clean up mounted image, err: %v", err)
		}
	}()

	report, err := c.ScanFilesystem(ctx, imagePath, scanOptions, false)
	if err != nil {
		return nil, fmt.Errorf("unable to scan image %s, err: %w", imgMeta.ID, err)
	}

	return report, err
}

func extractLayersFromOverlayFSMounts(mounts []mount.Mount) []string {
	var layers []string
	for _, mount := range mounts {
		for _, opt := range mount.Options {
			for _, prefix := range []string{"upperdir=", "lowerdir="} {
				trimmedOpt := strings.TrimPrefix(opt, prefix)
				if trimmedOpt != opt {
					layers = append(layers, strings.Split(trimmedOpt, ":")...)
				}
			}
		}
	}
	return layers
}
