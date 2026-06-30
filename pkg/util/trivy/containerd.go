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
	"slices"
	"strings"
	"time"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/images/archive"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/core/snapshots"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	refdocker "github.com/distribution/reference"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	dimage "github.com/moby/moby/api/types/image"
	dclient "github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errLayerChainMismatch fires when the snapshotter's parent chain for an
// image's top chainID does not match the chain we computed from
// rootfs.diff_ids. We refuse to scan rather than silently pair the wrong
// path with each DiffID.
var errLayerChainMismatch = errors.New("snapshotter chain does not match image config")

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
	return func(ctx context.Context, ref []string, _ ...dclient.ImageSaveOption) (dclient.ImageSaveResult, error) {
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

	portSet := make(map[string]struct{})
	for k := range imgConfig.Config.ExposedPorts {
		portSet[k] = struct{}{}
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
		Config: &dockerspec.DockerOCIImageConfig{
			ImageConfig: ocispec.ImageConfig{
				User:         imgConfig.Config.User,
				ExposedPorts: portSet,
				Env:          imgConfig.Config.Env,
				Cmd:          imgConfig.Config.Cmd,
				Volumes:      imgConfig.Config.Volumes,
				WorkingDir:   imgConfig.Config.WorkingDir,
				Entrypoint:   imgConfig.Config.Entrypoint,
				Labels:       imgConfig.Config.Labels,
			},
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

const defaultExpiration = 1 * time.Minute

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
	expiration := defaultExpiration
	if deadline, ok := ctx.Deadline(); ok {
		expiration = time.Until(deadline)
	}
	imageID := imgMeta.ID

	// img.RootFS, images.Manifest and SnapshotService all read from the
	// content store and snapshotter, which containerd indexes per
	// namespace. The outer ctx may not yet carry one.
	ctx = namespaces.WithNamespace(ctx, imgMeta.Namespace)

	mounts, snapshotter, cleanLease, err := client.MountsWithSnapshotter(ctx, expiration, imgMeta.Namespace, img)
	if err != nil {
		return nil, fmt.Errorf("unable to get mounts for image %s, err: %w", imgMeta.ID, err)
	}
	defer func() {
		if err := cleanLease(ctx); err != nil {
			log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
		}
	}()

	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to read diff_ids for image %s: %w", imgMeta.ID, err)
	}
	manifest, err := images.Manifest(ctx, img.ContentStore(), img.Target(), img.Platform())
	if err != nil {
		return nil, fmt.Errorf("unable to read manifest for image %s: %w", imgMeta.ID, err)
	}
	layers, err := buildContainerdLayerPaths(ctx, client.RawClient().SnapshotService(snapshotter), img.Name(), diffIDs, manifest, mounts)
	if err != nil {
		return nil, fmt.Errorf("unable to pair layer paths for image %s: %w", imgMeta.ID, err)
	}

	report, err := c.scanOverlayFS(ctx, lo.Map(layers, func(l ftypes.LayerPath, _ int) string { return l.Path }), &fakeContainerdContainer{
		image:         fanalImage,
		fakeContainer: newFakeContainer(layers, imgMeta),
	}, imgMeta, scanOptions)

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
	for _, mnt := range mounts {
		var fromOverlay bool
		for _, opt := range mnt.Options {
			for _, prefix := range []string{"upperdir=", "lowerdir="} {
				trimmedOpt := strings.TrimPrefix(opt, prefix)
				if trimmedOpt != opt {
					layers = append(layers, strings.Split(trimmedOpt, ":")...)
					fromOverlay = true
				}
			}
		}
		// A single-layer image is exposed by containerd as a single bind mount
		// (no lowerdir/upperdir overlay options); its only layer is the source.
		if !fromOverlay && mnt.Type == "bind" && mnt.Source != "" {
			layers = append(layers, mnt.Source)
		}
	}
	return layers
}

// computeChainIDs returns the chainID at each level of diffIDs, in
// image-config (bottom-up) order. It copies first because
// identity.ChainIDs mutates its argument in place.
func computeChainIDs(diffIDs []digest.Digest) []digest.Digest {
	if len(diffIDs) == 0 {
		return nil
	}
	out := slices.Clone(diffIDs)
	identity.ChainIDs(out)
	return out
}

// snapshotterStat is the slice of snapshots.Snapshotter we use, kept
// small so tests can stub it without the whole snapshotter surface.
type snapshotterStat interface {
	Stat(ctx context.Context, key string) (snapshots.Info, error)
}

// verifyChainAgainstSnapshotter confirms the snapshotter's stored parent
// chain matches chainIDs, catching a snapshotter / image-config
// disagreement before we pair LayerPaths off it.
func verifyChainAgainstSnapshotter(ctx context.Context, s snapshotterStat, chainIDs []digest.Digest) error {
	for i := len(chainIDs) - 1; i >= 0; i-- {
		info, err := s.Stat(ctx, chainIDs[i].String())
		if err != nil {
			return fmt.Errorf("snapshotter stat for %s: %w", chainIDs[i], err)
		}
		var wantParent string
		if i > 0 {
			wantParent = chainIDs[i-1].String()
		}
		if info.Parent != wantParent {
			return fmt.Errorf("%w: chainID %s has Parent=%q, expected %q",
				errLayerChainMismatch, chainIDs[i], info.Parent, wantParent)
		}
	}
	return nil
}

// buildContainerdLayerPaths returns one LayerPath per layer in
// image-config (bottom-up) order. It takes resolved OCI inputs rather
// than a containerd.Image so it stays testable. The manifest can
// legally have a different length than diff_ids; when it does, Digest
// is left empty rather than risk pairing a wrong one.
func buildContainerdLayerPaths(
	ctx context.Context,
	s snapshotterStat,
	imgName string,
	diffIDs []digest.Digest,
	manifest ocispec.Manifest,
	mounts []mount.Mount,
) ([]ftypes.LayerPath, error) {
	if len(diffIDs) == 0 {
		return nil, fmt.Errorf("image %s has no diff_ids", imgName)
	}
	if err := verifyChainAgainstSnapshotter(ctx, s, computeChainIDs(diffIDs)); err != nil {
		return nil, err
	}

	topDown := extractLayersFromOverlayFSMounts(mounts)
	if len(topDown) != len(diffIDs) {
		return nil, fmt.Errorf("%w: %d paths vs %d diff_ids", errLayerCountMismatch, len(topDown), len(diffIDs))
	}

	digestsAligned := len(manifest.Layers) == len(diffIDs)
	if !digestsAligned {
		log.Warnf("image %s: manifest has %d layers, diff_ids has %d; emitting SBOM without LayerDigest",
			imgName, len(manifest.Layers), len(diffIDs))
	}

	out := make([]ftypes.LayerPath, len(diffIDs))
	for i := range diffIDs {
		lp := ftypes.LayerPath{
			DiffID: diffIDs[i].String(),
			// overlay lowerdir is top-down; flip to image-config bottom-up.
			Path: topDown[len(topDown)-1-i],
		}
		if digestsAligned {
			lp.Digest = manifest.Layers[i].Digest.String()
		}
		out[i] = lp
	}
	return out, nil
}
