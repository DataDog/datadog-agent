// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package oci provides a way to interact with Datadog Packages OCIs.
package oci

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"golang.org/x/net/http2"
	httptrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/tar"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// RegistryAuthDefault is the default registry authentication method. Under the hood, it uses the Docker configuration.
	RegistryAuthDefault string = "docker"
	// RegistryAuthGCR is the Google Container Registry authentication method.
	RegistryAuthGCR string = "gcr"
	// RegistryAuthECR is the Amazon Elastic Container Registry authentication method.
	RegistryAuthECR string = "ecr"
)

const (
	// AnnotationPackage is the annotiation used to identify the package name.
	AnnotationPackage = "com.datadoghq.package.name"
	// AnnotationVersion is the annotiation used to identify the package version.
	AnnotationVersion = "com.datadoghq.package.version"
	// AnnotationSize is the annotiation used to identify the package size.
	AnnotationSize = "com.datadoghq.package.size"

	// DatadogPackageLayerMediaType is the media type for the main Datadog Package layer.
	DatadogPackageLayerMediaType types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	// DatadogPackageConfigLayerMediaType is the media type for the optional Datadog Package config layer.
	DatadogPackageConfigLayerMediaType types.MediaType = "application/vnd.datadog.package.config.layer.v1.tar+zstd"
)

const (
	layerMaxSize        = 3 << 30 // 3GiB
	extractLayerRetries = 3
)

// DownloadedPackage is the downloaded package.
type DownloadedPackage struct {
	Image   oci.Image
	Name    string
	Version string
	Size    uint64
}

// Downloader is the Downloader used by the installer to download packages.
type Downloader struct {
	env    *env.Env
	client *http.Client
}

// NewDownloader returns a new Downloader.
func NewDownloader(env *env.Env, client *http.Client) *Downloader {
	return &Downloader{
		env:    env,
		client: client,
	}
}

// Download downloads the Datadog Package referenced in the given Package struct.
func (d *Downloader) Download(ctx context.Context, packageURL string) (*DownloadedPackage, error) {
	log.Debugf("Downloading package from %s", packageURL)
	url, err := url.Parse(packageURL)
	if err != nil {
		return nil, fmt.Errorf("could not parse package URL: %w", err)
	}
	var image oci.Image
	switch url.Scheme {
	case "oci":
		image, err = d.downloadRegistry(ctx, strings.TrimPrefix(packageURL, "oci://"))
	case "file":
		image, err = d.downloadFile(url.Path)
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
	name, ok := manifest.Annotations[AnnotationPackage]
	if !ok {
		return nil, fmt.Errorf("package manifest is missing package annotation")
	}
	version, ok := manifest.Annotations[AnnotationVersion]
	if !ok {
		return nil, fmt.Errorf("package manifest is missing version annotation")
	}
	size := uint64(0)
	rawSize, ok := manifest.Annotations[AnnotationSize]
	if ok {
		size, err = strconv.ParseUint(rawSize, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse package size: %w", err)
		}
	}
	log.Debugf("Successfully downloaded package from %s", packageURL)
	return &DownloadedPackage{
		Image:   image,
		Name:    name,
		Version: version,
		Size:    size,
	}, nil
}

func getKeychain(auth string) authn.Keychain {
	switch auth {
	case RegistryAuthGCR:
		return google.Keychain
	case RegistryAuthECR:
		return authn.NewKeychainFromHelper(ecr.NewECRHelper())
	case RegistryAuthDefault, "":
		return authn.DefaultKeychain
	default:
		log.Warnf("unsupported registry authentication method: %s, defaulting to docker", auth)
		return authn.DefaultKeychain
	}
}

type urlWithKeychain struct {
	ref      string
	keychain authn.Keychain
}

// getRefAndKeychain returns the reference and keychain for the given URL.
// This function applies potential registry and authentication overrides set either globally or per image.
func getRefAndKeychain(env *env.Env, url string) urlWithKeychain {
	imageWithIdentifier := url[strings.LastIndex(url, "/")+1:]
	registryOverride := env.RegistryOverride
	for image, override := range env.RegistryOverrideByImage {
		if strings.HasPrefix(imageWithIdentifier, image+":") || strings.HasPrefix(imageWithIdentifier, image+"@") {
			registryOverride = override
			break
		}
	}
	ref := url
	if registryOverride != "" {
		if !strings.HasSuffix(registryOverride, "/") {
			registryOverride += "/"
		}
		ref = registryOverride + imageWithIdentifier
	}
	keychain := getKeychain(env.RegistryAuthOverride)
	for image, override := range env.RegistryAuthOverrideByImage {
		if strings.HasPrefix(imageWithIdentifier, image+":") || strings.HasPrefix(imageWithIdentifier, image+"@") {
			keychain = getKeychain(override)
			break
		}
	}
	return urlWithKeychain{
		ref:      ref,
		keychain: keychain,
	}
}

func (d *Downloader) downloadRegistry(ctx context.Context, url string) (oci.Image, error) {
	refAndKeychain := getRefAndKeychain(d.env, url)
	ref, err := name.ParseReference(refAndKeychain.ref)
	if err != nil {
		return nil, fmt.Errorf("could not parse reference: %w", err)
	}
	index, err := remote.Index(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(refAndKeychain.keychain), remote.WithTransport(httptrace.WrapRoundTripper(d.client.Transport)))
	if err != nil {
		return nil, fmt.Errorf("could not download image: %w", err)
	}
	return d.downloadIndex(index)
}

func (d *Downloader) downloadFile(path string) (oci.Image, error) {
	layoutPath, err := layout.FromPath(path)
	if err != nil {
		return nil, fmt.Errorf("could not get layout from path: %w", err)
	}
	imageIndex, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("could not get image index: %w", err)
	}
	return d.downloadIndex(imageIndex)
}

func (d *Downloader) downloadIndex(index oci.ImageIndex) (oci.Image, error) {
	platform := oci.Platform{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
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

// ExtractLayers extracts the layers of the downloaded package with the given media type to the given directory.
func (d *DownloadedPackage) ExtractLayers(mediaType types.MediaType, dir string) error {
	layers, err := d.Image.Layers()
	if err != nil {
		return fmt.Errorf("could not get image layers: %w", err)
	}
	for _, layer := range layers {
		layerMediaType, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("could not get layer media type: %w", err)
		}
		if layerMediaType == mediaType {
			// Retry stream reset errors
			for i := 0; i < extractLayerRetries; i++ {
				if i > 0 {
					time.Sleep(time.Second)
				}
				uncompressedLayer, err := layer.Uncompressed()
				if err != nil {
					return fmt.Errorf("could not uncompress layer: %w", err)
				}
				err = tar.Extract(uncompressedLayer, dir, layerMaxSize)
				uncompressedLayer.Close()
				if err != nil {
					if !isStreamResetError(err) {
						return fmt.Errorf("could not extract layer: %w", err)
					}
					log.Warnf("stream error while extracting layer, retrying")
					// Clean up the directory before retrying to avoid partial extraction
					err = tar.Clean(dir)
					if err != nil {
						return fmt.Errorf("could not clean directory: %w", err)
					}
				} else {
					break
				}
			}
		}
	}
	return nil
}

// WriteOCILayout writes the image as an OCI layout to the given directory.
func (d *DownloadedPackage) WriteOCILayout(dir string) error {
	layoutPath, err := layout.Write(dir, empty.Index)
	if err != nil {
		return fmt.Errorf("could not write layout: %w", err)
	}
	err = layoutPath.AppendImage(d.Image)
	if err != nil {
		return fmt.Errorf("could not append image to layout: %w", err)
	}
	return nil
}

// PackageURL returns the package URL for the given site, package and version.
func PackageURL(env *env.Env, pkg string, version string) string {
	switch env.Site {
	case "datad0g.com":
		return fmt.Sprintf("oci://docker.io/datadog/%s-package-dev:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	default:
		return fmt.Sprintf("oci://gcr.io/datadoghq/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	}
}

// isStreamResetError returns true if the given error is a stream reset error.
// Sometimes, in GCR, the tar extract fails with "stream error: stream ID x; INTERNAL_ERROR; received from peer".
// This happens because the uncompressed layer reader is a http/2 response body under the hood. That body is
// streamed and receives a "reset stream frame", with the code 0x2 (INTERNAL_ERROR). This is an error from the server
// that we need to retry.
func isStreamResetError(err error) bool {
	if err == nil {
		return false
	}
	serr := http2.StreamError{}
	if errors.As(err, &serr) {
		return serr.Code == http2.ErrCodeInternal
	}
	serrp := &http2.StreamError{}
	if errors.As(err, &serrp) {
		return serrp.Code == http2.ErrCodeInternal
	}
	return false
}
