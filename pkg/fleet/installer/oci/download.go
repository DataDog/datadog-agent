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
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"go.uber.org/multierr"
	"golang.org/x/net/http2"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/tar"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// RegistryAuthDefault is the default registry authentication method. Under the hood, it uses the Docker configuration.
	RegistryAuthDefault string = "docker"
	// RegistryAuthGCR is the Google Container Registry authentication method.
	RegistryAuthGCR string = "gcr"
	// RegistryAuthPassword is the password registry authentication method.
	RegistryAuthPassword string = "password"
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
	// DatadogPackageInstallerLayerMediaType is the media type for the optional Datadog Package installer layer.
	DatadogPackageInstallerLayerMediaType types.MediaType = "application/vnd.datadog.package.installer.layer.v1"
)

const (
	layerMaxSize   = 3 << 30 // 3GiB
	networkRetries = 3
)

var (
	defaultRegistriesStaging = []string{
		"install.datad0g.com",
	}
	defaultRegistriesProd = []string{
		"install.datadoghq.com",
		"gcr.io/datadoghq",
	}
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
		return nil, fmt.Errorf("could not download package: %w", err)
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

func getKeychain(auth string, username string, password string) authn.Keychain {
	switch auth {
	case RegistryAuthGCR:
		return google.Keychain
	case RegistryAuthPassword:
		return usernamePasswordKeychain{
			username: username,
			password: password,
		}
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

// getRefAndKeychains returns the references and their keychains to try in order to download an OCI at the given URL
func getRefAndKeychains(mainEnv *env.Env, url string) []urlWithKeychain {
	mainRefAndKeyChain := getRefAndKeychain(mainEnv, url)
	refAndKeychains := []urlWithKeychain{mainRefAndKeyChain}
	if mainRefAndKeyChain.ref != url || mainRefAndKeyChain.keychain != authn.DefaultKeychain {
		// Override: we don't need to try the default registries
		return refAndKeychains
	}

	defaultRegistries := defaultRegistriesProd
	if mainEnv.Site == "datad0g.com" {
		defaultRegistries = defaultRegistriesStaging
	}
	for _, additionalDefaultRegistry := range defaultRegistries {
		refAndKeychain := getRefAndKeychain(&env.Env{RegistryOverride: additionalDefaultRegistry}, url)
		// Deduplicate
		found := false
		for _, rk := range refAndKeychains {
			if rk.ref == refAndKeychain.ref && rk.keychain == refAndKeychain.keychain {
				found = true
				break
			}
		}
		if !found {
			refAndKeychains = append(refAndKeychains, refAndKeychain)
		}
	}

	return refAndKeychains
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
	// public.ecr.aws/datadog is ignored for now as there are issues with it
	if registryOverride != "" && registryOverride != "public.ecr.aws/datadog" {
		if !strings.HasSuffix(registryOverride, "/") {
			registryOverride += "/"
		}
		ref = registryOverride + imageWithIdentifier
	}
	keychain := getKeychain(env.RegistryAuthOverride, env.RegistryUsername, env.RegistryPassword)
	for image, override := range env.RegistryAuthOverrideByImage {
		if strings.HasPrefix(imageWithIdentifier, image+":") || strings.HasPrefix(imageWithIdentifier, image+"@") {
			keychain = getKeychain(override, env.RegistryUsername, env.RegistryPassword)
			break
		}
	}
	return urlWithKeychain{
		ref:      ref,
		keychain: keychain,
	}
}

// downloadRegistry downloads the image from a remote registry.
// If they are specified, the registry and authentication overrides are applied first.
// Then we try each registry in the list of default registries in order and return the first successful download.
func (d *Downloader) downloadRegistry(ctx context.Context, url string) (oci.Image, error) {
	transport := telemetry.WrapRoundTripper(d.client.Transport)
	var err error
	if d.env.Mirror != "" {
		transport, err = newMirrorTransport(transport, d.env.Mirror)
		if err != nil {
			return nil, fmt.Errorf("could not create mirror transport: %w", err)
		}
	}
	var multiErr error
	for _, refAndKeychain := range getRefAndKeychains(d.env, url) {
		log.Debugf("Downloading index from %s", refAndKeychain.ref)
		ref, err := name.ParseReference(refAndKeychain.ref)
		if err != nil {
			multiErr = multierr.Append(multiErr, fmt.Errorf("could not parse reference: %w", err))
			log.Warnf("could not parse reference: %s", err.Error())
			continue
		}
		index, err := remote.Index(
			ref,
			remote.WithContext(ctx),
			remote.WithAuthFromKeychain(refAndKeychain.keychain),
			remote.WithTransport(transport),
		)
		if err != nil {
			multiErr = multierr.Append(multiErr, fmt.Errorf("could not download image using %s: %w", url, err))
			log.Warnf("could not download image using %s: %s", url, err.Error())
			continue
		}
		return d.downloadIndex(index)
	}
	return nil, fmt.Errorf("could not download image from any registry: %w", multiErr)
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
	return nil, installerErrors.Wrap(
		installerErrors.ErrPackageNotFound,
		fmt.Errorf("no matching image found in the index"),
	)
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
			err = withNetworkRetries(
				func() error {
					var err error
					defer func() {
						if err != nil {
							deferErr := tar.Clean(dir)
							if deferErr != nil {
								err = deferErr
							}
						}
					}()
					uncompressedLayer, err := layer.Uncompressed()
					if err != nil {
						return err
					}

					switch layerMediaType {
					case DatadogPackageLayerMediaType, DatadogPackageConfigLayerMediaType:
						err = tar.Extract(uncompressedLayer, dir, layerMaxSize)
					case DatadogPackageInstallerLayerMediaType:
						err = writeBinary(uncompressedLayer, dir)
					default:
						return fmt.Errorf("unsupported layer media type: %s", layerMediaType)
					}
					uncompressedLayer.Close()
					if err != nil {
						return err
					}
					return nil
				},
			)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		}
	}
	return nil
}

// WriteOCILayout writes the image as an OCI layout to the given directory.
func (d *DownloadedPackage) WriteOCILayout(dir string) (err error) {
	var layoutPath layout.Path
	return withNetworkRetries(
		func() error {
			layoutPath, err = layout.Write(dir, empty.Index)
			if err != nil {
				return fmt.Errorf("could not write layout: %w", err)
			}

			err = layoutPath.AppendImage(d.Image)
			if err != nil {
				return fmt.Errorf("could not append image to layout: %w", err)
			}
			return nil
		},
	)
}

// PackageURL returns the package URL for the given site, package and version.
func PackageURL(env *env.Env, pkg string, version string) string {
	switch env.Site {
	case "datad0g.com":
		return fmt.Sprintf("oci://install.datad0g.com/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	default:
		return fmt.Sprintf("oci://install.datadoghq.com/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	}
}

func withNetworkRetries(f func() error) error {
	var err error
	for i := 0; i < networkRetries; i++ {
		err = f()
		if err == nil {
			return nil
		}
		if !isRetryableNetworkError(err) {
			return err
		}
		log.Warnf("retrying after network error: %s", err)
		time.Sleep(time.Second)
	}
	return err
}

// isRetryableNetworkError returns true if the error is a network error we should retry on
func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}

	if netErr, ok := err.(*net.OpError); ok {
		if netErr.Temporary() {
			// Temporary errors, such as "connection timed out"
			return true
		}
		if syscallErr, ok := netErr.Err.(*os.SyscallError); ok {
			if errno, ok := syscallErr.Err.(syscall.Errno); ok {
				// Connection reset errors, such as "connection reset by peer"
				return errno == syscall.ECONNRESET
			}
		}
	}

	if strings.Contains(err.Error(), "connection reset by peer") {
		return true
	}

	return isStreamResetError(err)
}

// isStreamResetError returns true if the given error is a stream reset error.
// Sometimes, in GCR, the tar extract fails with "stream error: stream ID x; INTERNAL_ERROR; received from peer".
// This happens because the uncompressed layer reader is a http/2 response body under the hood. That body is
// streamed and receives a "reset stream frame", with the code 0x2 (INTERNAL_ERROR). This is an error from the server
// that we need to retry.
func isStreamResetError(err error) bool {
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

type usernamePasswordKeychain struct {
	username string
	password string
}

func (k usernamePasswordKeychain) Resolve(_ authn.Resource) (authn.Authenticator, error) {
	return authn.FromConfig(authn.AuthConfig{
		Username: k.username,
		Password: k.password,
	}), nil
}

// writeBinary extracts the binary from the given reader to the given path.
func writeBinary(r io.Reader, path string) error {
	// Ensure the file has 0700 permissions even if it already exists
	if err := os.Chmod(path, 0700); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not set file permissions before writing: %w", err)
	}
	outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer outFile.Close()

	// Now that we have the 0700 permissions set, we can write to the file.
	// Use io.LimitReader to limit the size of the layer to layerMaxSize.
	limitedReader := io.LimitReader(r, layerMaxSize)
	_, err = io.Copy(outFile, limitedReader)
	if err != nil {
		return fmt.Errorf("could not write to file: %w", err)
	}

	return nil
}
