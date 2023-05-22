// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	refdocker "github.com/containerd/containerd/reference/docker"
	api "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

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
	return func(ctx context.Context, ref []string) (io.ReadCloser, error) {
		if len(ref) < 1 {
			return nil, xerrors.New("no image reference")
		}
		imgOpts := archive.WithImage(client.ImageService(), ref[0])
		manifestOpts := archive.WithManifest(img.Target())
		platOpts := archive.WithPlatform(platforms.DefaultStrict())
		pr, pw := io.Pipe()
		go func() {
			pw.CloseWithError(archive.Export(ctx, client.ContentStore(), pw, imgOpts, manifestOpts, platOpts))
		}()
		return pr, nil
	}
}

// Custom code based on https://github.com/aquasecurity/trivy/blob/2206e008ea6e5f4e5c1aa7bc8fc77dae7041de6a/pkg/fanal/image/daemon/containerd.go `ContainerdImage`
func convertContainerdImage(ctx context.Context, client *containerd.Client, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (types.Image, func(), error) {
	ctx = namespaces.WithNamespace(ctx, imgMeta.Namespace)
	cleanup := func() {}

	f, err := os.CreateTemp("", "fanal-containerd-*")
	if err != nil {
		return nil, cleanup, xerrors.Errorf("failed to create a temporary file: %w", err)
	}

	cleanup = func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}

	insp, history, ref, err := inspect(ctx, imgMeta, img)
	if err != nil {
		return nil, cleanup, xerrors.Errorf("inspect error: %w", err) // Note: the original code doesn't return "cleanup".
	}

	return &image{
		name:    img.Name(),
		opener:  imageOpener(ctx, ref.String(), f, imageWriter(client, img)),
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
func inspect(ctx context.Context, imgMeta *workloadmeta.ContainerImageMetadata, img containerd.Image) (api.ImageInspect, []v1.History, refdocker.Reference, error) {
	ref := familiarNamed(img.Name())

	imgConfig, imgConfigDesc, err := readImageConfig(ctx, img)
	if err != nil {
		return api.ImageInspect{}, nil, nil, err
	}

	var lastHistory ocispec.History
	if len(imgConfig.History) > 0 {
		lastHistory = imgConfig.History[len(imgConfig.History)-1]
	}

	var history []v1.History
	for _, h := range imgConfig.History {
		history = append(history, v1.History{
			Author:     h.Author,
			Created:    v1.Time{Time: *h.Created},
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

	return api.ImageInspect{
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
		RootFS: api.RootFS{
			Type: imgConfig.RootFS.Type,
			Layers: lo.Map(imgConfig.RootFS.DiffIDs, func(d digest.Digest, _ int) string {
				return d.String()
			}),
		},
	}, history, ref, nil
}
