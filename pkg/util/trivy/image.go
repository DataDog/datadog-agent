// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Imported from https://github.com/aquasecurity/trivy/blob/main/pkg/fanal/image/daemon/image.go

//go:build trivy
// +build trivy

package trivy

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	fimage "github.com/aquasecurity/trivy/pkg/fanal/image"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/samber/lo"
	"golang.org/x/xerrors"
)

var mu sync.Mutex

type opener func() (v1.Image, error)

type imageSave func(context.Context, []string) (io.ReadCloser, error)

func imageOpener(ctx context.Context, ref string, f *os.File, imageSave imageSave) opener {
	return func() (v1.Image, error) {
		// Store the tarball in local filesystem and return a new reader into the bytes each time we need to access something.
		rc, err := imageSave(ctx, []string{ref})
		if err != nil {
			return nil, xerrors.Errorf("unable to export the image: %w", err)
		}
		defer rc.Close()

		if _, err = io.Copy(f, rc); err != nil {
			return nil, xerrors.Errorf("failed to copy the image: %w", err)
		}
		defer f.Close()

		img, err := tarball.ImageFromPath(f.Name(), nil)
		if err != nil {
			return nil, xerrors.Errorf("failed to initialize the struct from the temporary file: %w", err)
		}

		return img, nil
	}
}

// image is a wrapper for github.com/google/go-containerregistry/pkg/v1/daemon.Image
// daemon.Image loads the entire image into the memory at first,
// but it doesn't need to load it if the information is already in the cache,
// To avoid entire loading, this wrapper uses ImageInspectWithRaw and checks image ID and layer IDs.
type image struct {
	v1.Image
	name    string
	opener  opener
	inspect types.ImageInspect
	history []v1.History
}

// populateImage initializes an "image" struct.
// This method is called by some goroutines at the same time.
// To prevent multiple heavy initializations, the lock is necessary.
func (img *image) populateImage() (err error) {
	mu.Lock()
	defer mu.Unlock()

	// img.Image is already initialized, so we don't have to do it again.
	if img.Image != nil {
		return nil
	}

	img.Image, err = img.opener()
	if err != nil {
		return xerrors.Errorf("unable to open: %w", err)
	}

	return nil
}

func (img *image) ConfigName() (v1.Hash, error) {
	return v1.NewHash(img.inspect.ID)
}

func (img *image) ConfigFile() (*v1.ConfigFile, error) {
	if len(img.inspect.RootFS.Layers) == 0 {
		// Podman doesn't return RootFS...
		return img.configFile()
	}

	nonEmptyLayerCount := lo.CountBy(img.history, func(history v1.History) bool {
		return !history.EmptyLayer
	})

	if len(img.inspect.RootFS.Layers) != nonEmptyLayerCount {
		// In cases where empty layers are not correctly determined from the history API.
		// There are some edge cases where we cannot guess empty layers well.
		return img.configFile()
	}

	diffIDs, err := img.diffIDs()
	if err != nil {
		return nil, xerrors.Errorf("unable to get diff IDs: %w", err)
	}

	created, err := time.Parse(time.RFC3339Nano, img.inspect.Created)
	if err != nil {
		return nil, xerrors.Errorf("failed parsing created %s: %w", img.inspect.Created, err)
	}

	return &v1.ConfigFile{
		Architecture:  img.inspect.Architecture,
		Author:        img.inspect.Author,
		Container:     img.inspect.Container,
		Created:       v1.Time{Time: created},
		DockerVersion: img.inspect.DockerVersion,
		Config:        img.imageConfig(img.inspect.Config),
		History:       img.history,
		OS:            img.inspect.Os,
		RootFS: v1.RootFS{
			Type:    img.inspect.RootFS.Type,
			DiffIDs: diffIDs,
		},
	}, nil
}

func (img *image) configFile() (*v1.ConfigFile, error) {
	// Need to fall back into expensive operations like "docker save"
	// because the config file cannot be generated properly from container engine API for some reason.
	if err := img.populateImage(); err != nil {
		return nil, xerrors.Errorf("unable to populate: %w", err)
	}
	return img.Image.ConfigFile()
}

func (img *image) LayerByDiffID(h v1.Hash) (v1.Layer, error) {
	if err := img.populateImage(); err != nil {
		return nil, xerrors.Errorf("unable to populate: %w", err)
	}
	return img.Image.LayerByDiffID(h)
}

func (img *image) RawConfigFile() ([]byte, error) {
	if err := img.populateImage(); err != nil {
		return nil, xerrors.Errorf("unable to populate: %w", err)
	}
	return img.Image.RawConfigFile()
}

func (img *image) RepoTags() []string {
	return img.inspect.RepoTags
}

func (img *image) RepoDigests() []string {
	return img.inspect.RepoDigests
}

func (img *image) diffIDs() ([]v1.Hash, error) {
	var diffIDs []v1.Hash
	for _, l := range img.inspect.RootFS.Layers {
		h, err := v1.NewHash(l)
		if err != nil {
			return nil, xerrors.Errorf("invalid hash %s: %w", l, err)
		}
		diffIDs = append(diffIDs, h)
	}
	return diffIDs, nil
}

func (img *image) imageConfig(config *container.Config) v1.Config {
	if config == nil {
		return v1.Config{}
	}

	c := v1.Config{
		AttachStderr:    config.AttachStderr,
		AttachStdin:     config.AttachStdin,
		AttachStdout:    config.AttachStdout,
		Cmd:             config.Cmd,
		Domainname:      config.Domainname,
		Entrypoint:      config.Entrypoint,
		Env:             config.Env,
		Hostname:        config.Hostname,
		Image:           config.Image,
		Labels:          config.Labels,
		OnBuild:         config.OnBuild,
		OpenStdin:       config.OpenStdin,
		StdinOnce:       config.StdinOnce,
		Tty:             config.Tty,
		User:            config.User,
		Volumes:         config.Volumes,
		WorkingDir:      config.WorkingDir,
		ArgsEscaped:     config.ArgsEscaped,
		NetworkDisabled: config.NetworkDisabled,
		MacAddress:      config.MacAddress,
		StopSignal:      config.StopSignal,
		Shell:           config.Shell,
	}

	if config.Healthcheck != nil {
		c.Healthcheck = &v1.HealthConfig{
			Test:        config.Healthcheck.Test,
			Interval:    config.Healthcheck.Interval,
			Timeout:     config.Healthcheck.Timeout,
			StartPeriod: config.Healthcheck.StartPeriod,
			Retries:     config.Healthcheck.Retries,
		}
	}

	if len(config.ExposedPorts) > 0 {
		c.ExposedPorts = map[string]struct{}{}
		for port := range c.ExposedPorts {
			c.ExposedPorts[port] = struct{}{}
		}
	}

	return c
}

func (img *image) Name() string {
	return img.name
}

func (img *image) ID() (string, error) {
	return fimage.ID(img)
}
