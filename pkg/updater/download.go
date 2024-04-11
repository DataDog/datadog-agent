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
	httptrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http"

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

type downloadedPackage struct {
	Image   oci.Image
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
func (d *downloader) Download(ctx context.Context, packageURL string) (*downloadedPackage, error) {
	log.Debugf("Downloading package from %s", packageURL)
	url, err := url.Parse(packageURL)
	if err != nil {
		return nil, fmt.Errorf("could not parse package URL: %w", err)
	}
	var image oci.Image
	switch url.Scheme {
	case "oci":
		image, err = d.downloadRegistry(ctx, d.getRegistryURL(packageURL))
	default:
		return nil, fmt.Errorf("unsupported package URL scheme: %s", url.Scheme)
	}
	if err != nil {
		return nil, fmt.Errorf("could not download package from %s: %w", packageURL, err)
	}
	manifest, err := image.Manifest()
	if err != nil {
		return nil, fmt.Errorf("could not get image manifest: %w", err)
	}
	name, ok := manifest.Annotations[annotationPackage]
	if !ok {
		return nil, fmt.Errorf("package manifest is missing package annotation")
	}
	version, ok := manifest.Annotations[annotationVersion]
	if !ok {
		return nil, fmt.Errorf("package manifest is missing version annotation")
	}
	log.Debugf("Successfully downloaded package from %s", packageURL)
	return &downloadedPackage{
		Image:   image,
		Name:    name,
		Version: version,
	}, nil
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
	index, err := remote.Index(ref, remote.WithContext(ctx), remote.WithTransport(httptrace.WrapRoundTripper(d.client.Transport)))
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
