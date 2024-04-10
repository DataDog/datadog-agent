// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ociLayoutArchiveName          = "oci-layout.tar"
	ociLayoutName                 = "oci-layout"
	ociLayoutArchiveMaxSize int64 = 1 << 30 // 1GiB
	ociLayoutMaxSize        int64 = 1 << 30 // 1GiB

	annotationPackage = "com.datadoghq.package.name"
	annotationVersion = "com.datadoghq.package.version"
)

type packageMetadata struct {
	Name    string
	Version string
}

// downloader is the downloader used by the updater to download packages.
type downloader struct {
	client        *http.Client
	remoteBaseURL string
}

// newDownloader returns a new Downloader.
func newDownloader(client *http.Client, remoteBaseURL string) *downloader {
	return &downloader{
		client:        client,
		remoteBaseURL: remoteBaseURL,
	}
}

// Download downloads the Datadog Package referenced in the given Package struct.
func (d *downloader) Download(ctx context.Context, pkg Package) (oci.Image, error) {
	log.Debugf("Downloading package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	url, err := url.Parse(pkg.URL)
	if err != nil {
		return nil, fmt.Errorf("could not parse package URL: %w", err)
	}
	var image oci.Image
	switch url.Scheme {
	case "oci":
		image, err = d.downloadRegistry(ctx, d.getRegistryURL(pkg.URL))
	default:
		return nil, fmt.Errorf("unsupported package URL scheme: %s", url.Scheme)
	}
	if err != nil {
		return nil, fmt.Errorf("could not download package from %s: %w", pkg.URL, err)
	}
	err = d.checkImageMetadata(image, pkg.Name, pkg.Version)
	if err != nil {
		return nil, fmt.Errorf("invalid package metadata: %w", err)
	}
	log.Debugf("Successfully downloaded package %s version %s from %s", pkg.Name, pkg.Version, pkg.URL)
	return image, nil
}

// Package returns the downloadable package at the given URL.
func (d *downloader) Package(ctx context.Context, pkgURL string) (Package, error) {
	log.Debugf("Getting package information from %s", pkgURL)
	url, err := url.Parse(pkgURL)
	if err != nil {
		return Package{}, fmt.Errorf("could not parse package URL: %w", err)
	}
	if url.Scheme != "oci" {
		return Package{}, fmt.Errorf("unsupported package URL scheme: %s", url.Scheme)
	}
	image, err := d.downloadRegistry(ctx, d.getRegistryURL(pkgURL))
	if err != nil {
		return Package{}, fmt.Errorf("could not download package from %s: %w", pkgURL, err)
	}
	metadata, err := d.imageMetadata(image)
	if err != nil {
		return Package{}, fmt.Errorf("could not get package metadata: %w", err)
	}
	return Package{
		Name:    metadata.Name,
		Version: metadata.Version,
		URL:     pkgURL,
	}, nil
}

func (d *downloader) imageMetadata(image oci.Image) (packageMetadata, error) {
	manifest, err := image.Manifest()
	if err != nil {
		return packageMetadata{}, fmt.Errorf("could not get image manifest: %w", err)
	}
	name, ok := manifest.Annotations[annotationPackage]
	if !ok {
		return packageMetadata{}, fmt.Errorf("package manifest is missing package annotation")
	}
	version, ok := manifest.Annotations[annotationVersion]
	if !ok {
		return packageMetadata{}, fmt.Errorf("package manifest is missing version annotation")
	}
	return packageMetadata{
		Name:    name,
		Version: version,
	}, nil
}

func (d *downloader) checkImageMetadata(image oci.Image, expectedName string, expectedVersion string) error {
	imageMetadata, err := d.imageMetadata(image)
	if err != nil {
		return fmt.Errorf("could not get image metadata: %w", err)
	}
	if imageMetadata.Name != expectedName || imageMetadata.Version != expectedVersion {
		return fmt.Errorf("invalid image metadata: expected %s version %s, got %s version %s", expectedName, expectedVersion, imageMetadata.Name, imageMetadata.Version)
	}
	return nil
}

func (d *downloader) getRegistryURL(url string) string {
	downloadURL := strings.TrimPrefix(url, "oci://")
	if d.remoteBaseURL != "" {
		remoteBaseURL := d.remoteBaseURL
		if !strings.HasSuffix(d.remoteBaseURL, "/") {
			remoteBaseURL += "/"
		}
		split := strings.Split(url, "/")
		downloadURL = remoteBaseURL + split[len(split)-1]
	}
	return downloadURL
}

func (d *downloader) downloadRegistry(ctx context.Context, url string) (oci.Image, error) {
	// the image URL is parsed as a digest to ensure we use the <repository>/<image>@<digest> format
	ref, err := name.ParseReference(url, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("could not parse ref: %w", err)
	}
	platform := oci.Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
	index, err := remote.Index(ref, remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("could not download image: %w", err)
	}
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("could not get index manifest: %w", err)
	}
	for _, manifest := range indexManifest.Manifests {
		if manifest.Platform != nil && !manifest.Platform.Satisfies(platform) {
			continue
		}
		image, err := index.Image(manifest.Digest)
		if err != nil {
			return nil, fmt.Errorf("could not get image: %w", err)
		}
		return image, nil
	}
	return nil, fmt.Errorf("no matching image found in the index")
}
