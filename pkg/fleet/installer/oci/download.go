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

	// VariantFIPS is the value used in oci.Platform.Variant to mark a FIPS-compliant
	// build. The OCI spec defines variant for CPU variants; Datadog overloads it as a
	// build flavor to distinguish sibling manifests for the same os/arch within a single
	// OCI index.
	VariantFIPS = "fips"

	// DatadogPackageLayerMediaType is the media type for the main Datadog Package layer.
	DatadogPackageLayerMediaType types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	// DatadogPackageConfigLayerMediaType is the media type for the optional Datadog Package config layer.
	DatadogPackageConfigLayerMediaType types.MediaType = "application/vnd.datadog.package.config.layer.v1.tar+zstd"
	// DatadogPackageInstallerLayerMediaType is the media type for the optional Datadog Package installer layer.
	DatadogPackageInstallerLayerMediaType types.MediaType = "application/vnd.datadog.package.installer.layer.v1"
	// DatadogPackageExtensionLayerMediaType is the media type for the optional Datadog Package extension layer.
	DatadogPackageExtensionLayerMediaType types.MediaType = "application/vnd.datadog.package.extension.layer.v1.tar+zstd"
)

// ErrNoLayerMatchesAnnotations is the error returned when no layer matches the requested annotations.
var ErrNoLayerMatchesAnnotations = errors.New("no layer matches the requested annotations")

// RegistryError annotates an error returned while talking to a specific OCI
// registry. When Downloader.Download has to try multiple registries, each
// per-registry failure is wrapped in a RegistryError so callers can iterate
// them (see RegistryErrors) and present each attempt to the user with its
// registry context — similar to how os.LinkError carries the path alongside
// the underlying error.
//
// Registry is the ref returned by getRefAndKeychain, which guarantees no
// embedded userinfo (formatImageRef strips it). Callers do not need to
// re-redact before logging or exporting Registry.
type RegistryError struct {
	Registry string // the registry URL / reference that was attempted
	Err      error  // the underlying error returned by that registry
}

// Error implements the error interface.
func (e *RegistryError) Error() string {
	return fmt.Sprintf("%s: %v", e.Registry, e.Err)
}

// Unwrap lets errors.Is / errors.As reach the underlying error (e.g. a
// go-containerregistry *transport.Error, a *net.DNSError, etc.).
func (e *RegistryError) Unwrap() error {
	return e.Err
}

// RegistryErrors walks err and returns all *RegistryError values found in its
// chain, across multierr branches. Useful for presenting per-registry failure
// summaries when a multi-registry download fails. Returns an empty slice if
// there are no RegistryError values.
func RegistryErrors(err error) []*RegistryError {
	var out []*RegistryError
	collectRegistryErrors(err, &out)
	return out
}

func collectRegistryErrors(err error, out *[]*RegistryError) {
	if err == nil {
		return
	}
	if re, ok := err.(*RegistryError); ok {
		*out = append(*out, re)
		// Don't descend further: a RegistryError's Err is the underlying
		// transport / net error, not another RegistryError.
		return
	}
	// multierr (and any other Unwrap-returns-slice error) — walk each child.
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range u.Unwrap() {
			collectRegistryErrors(e, out)
		}
		return
	}
	// Single-wrap chains.
	if u, ok := err.(interface{ Unwrap() error }); ok {
		collectRegistryErrors(u.Unwrap(), out)
	}
}

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

// LayerAnnotation is the annotation used to identify the layer.
type LayerAnnotation struct {
	Key   string
	Value string
}

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

// WithRegistryOverride returns a new Downloader with the given registry overrides applied.
// The returned Downloader shares the same HTTP client as the original.
// Image-scoped override maps (from DD_INSTALLER_REGISTRY_URL_<IMAGE> env vars) are
// preserved and take precedence over the per-extension override, matching the
// existing priority order.
func (d *Downloader) WithRegistryOverride(url, auth, username, password string) *Downloader {
	envCopy := *d.env
	if url != "" {
		envCopy.RegistryOverride = url
	}
	if auth != "" {
		envCopy.RegistryAuthOverride = auth
	}
	if username != "" {
		envCopy.RegistryUsername = username
	}
	if password != "" {
		envCopy.RegistryPassword = password
	}
	return &Downloader{
		env:    &envCopy,
		client: d.client,
	}
}

// Download downloads the Datadog Package referenced in the given Package struct.
func (d *Downloader) Download(ctx context.Context, packageURL string) (_ *DownloadedPackage, err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "oci.download")
	defer func() { span.Finish(err) }()
	span.SetTag("package.url", packageURL)
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
		return nil, errors.New("package manifest is missing package annotation")
	}
	version, ok := manifest.Annotations[AnnotationVersion]
	if !ok {
		return nil, errors.New("package manifest is missing version annotation")
	}
	size := uint64(0)
	rawSize, ok := manifest.Annotations[AnnotationSize]
	if ok {
		size, err = strconv.ParseUint(rawSize, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse package size: %w", err)
		}
	}
	span.SetTag("package.name", name)
	span.SetTag("package.version", version)
	span.SetTag("package.size", int64(size))
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
		registryOverride = formatImageRef(registryOverride)
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

// formatImageRef formats the image ref by removing the http:// or https://
// prefix and dropping any embedded userinfo. Credentials must be supplied via
// the dedicated RegistryUsername / RegistryPassword config fields; userinfo
// in the URL is unsupported and would otherwise leak into logs and spans.
func formatImageRef(override string) string {
	override = strings.TrimPrefix(strings.TrimPrefix(override, "https://"), "http://")
	// Parse with a scheme so url.Parse populates User; bare host[:port]/path
	// inputs would otherwise be treated as opaque.
	u, err := url.Parse("https://" + override)
	if err != nil || u.User == nil {
		return override
	}
	log.Warnf("ignoring userinfo in registry override URL; use installer.registry.username / installer.registry.password instead")
	u.User = nil
	return strings.TrimPrefix(u.String(), "https://")
}

// downloadRegistry downloads the image from a remote registry.
// If they are specified, the registry and authentication overrides are applied first.
// Then we try each registry in the list of default registries in order and return the first successful download.
func (d *Downloader) downloadRegistry(ctx context.Context, rawURL string) (oci.Image, error) {
	transport := telemetry.WrapRoundTripper(d.client.Transport)
	var err error
	if d.env.Mirror != "" {
		transport, err = newMirrorTransport(transport, d.env.Mirror)
		if err != nil {
			return nil, fmt.Errorf("could not create mirror transport: %w", err)
		}
	}
	var multiErr error
	for _, refAndKeychain := range getRefAndKeychains(d.env, rawURL) {
		log.Debugf("Downloading index from %s", refAndKeychain.ref)
		ref, err := name.ParseReference(refAndKeychain.ref)
		if err != nil {
			multiErr = multierr.Append(multiErr, &RegistryError{Registry: refAndKeychain.ref, Err: err})
			log.Debugf("could not parse reference: %s", err.Error())
			continue
		}
		index, err := remote.Index(
			ref,
			remote.WithContext(ctx),
			remote.WithAuthFromKeychain(refAndKeychain.keychain),
			remote.WithTransport(transport),
		)
		if err != nil {
			multiErr = multierr.Append(multiErr, &RegistryError{Registry: refAndKeychain.ref, Err: err})
			log.Debugf("could not download image using %s: %s", rawURL, err.Error())
			continue
		}
		if span, ok := telemetry.SpanFromContext(ctx); ok {
			span.SetTag("registry.ref", refAndKeychain.ref)
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
	desiredVariant := ""
	if d.env.FIPSMode {
		desiredVariant = VariantFIPS
		platform.Variant = VariantFIPS
	}
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("could not get index manifest: %w", err)
	}
	for _, manifest := range indexManifest.Manifests {
		if manifest.Platform != nil && !manifest.Platform.Satisfies(platform) {
			continue
		}
		// Platform.Satisfies treats an empty required Variant as a wildcard, so
		// the non-FIPS path could otherwise accept a FIPS-tagged manifest
		// depending on index order. Filter to an exact variant match.
		if manifest.Platform != nil && manifest.Platform.Variant != desiredVariant {
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
		errors.New("no matching image found in the index"),
	)
}

// ExtractLayers extracts the layers of the downloaded package with the given media type to the given directory.
func (d *DownloadedPackage) ExtractLayers(ctx context.Context, mediaType types.MediaType, dir string, annotationFilters ...LayerAnnotation) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "oci.extract_layers")
	defer func() { span.Finish(err) }()
	span.SetTag("media.type", string(mediaType))
	span.SetTag("package.name", d.Name)
	totalAttempts := 0
	totalCompressedSize := int64(0)
	layerCount := 0
	defer func() {
		span.SetTag("network_attempts", totalAttempts)
		span.SetTag("layer_size", totalCompressedSize)
		span.SetTag("layer_count", layerCount)
	}()

	manifest, err := d.Image.Manifest()
	if err != nil {
		return fmt.Errorf("could not get image manifest: %w", err)
	}
	matchesAnnotationsCount := 0
	for _, layerManifest := range manifest.Layers {
		if layerManifest.MediaType != mediaType {
			continue
		}

		matchesAnnotations := true
		for _, annotationFilter := range annotationFilters {
			if layerManifest.Annotations[annotationFilter.Key] != annotationFilter.Value {
				matchesAnnotations = false
				break
			}
		}
		if !matchesAnnotations {
			continue
		}
		matchesAnnotationsCount++
		layerCount++
		totalCompressedSize += layerManifest.Size

		layer, err := d.Image.LayerByDigest(layerManifest.Digest)
		if err != nil {
			return fmt.Errorf("could not get layer: %w", err)
		}
		attempts, err := extractLayer(ctx, d.Name, layerManifest, layer, dir)
		totalAttempts += attempts
		if err != nil {
			return fmt.Errorf("could not extract layer: %w", err)
		}
	}
	if matchesAnnotationsCount == 0 && len(annotationFilters) > 0 {
		return ErrNoLayerMatchesAnnotations
	}
	return nil
}

// extractLayer extracts a single layer to dir, with retries on transient network errors.
// Returns the total number of attempts made (1 if it succeeded on the first try).
func extractLayer(ctx context.Context, pkgName string, layerManifest oci.Descriptor, layer oci.Layer, dir string) (attempts int, err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "oci.extract_layer")
	defer func() { span.Finish(err) }()
	span.SetTag("package.name", pkgName)
	resource := string(layerManifest.MediaType)
	if extName := layerManifest.Annotations["com.datadoghq.package.extension.name"]; extName != "" {
		resource = resource + "/" + extName
		span.SetTag("extension.name", extName)
	}
	span.SetResourceName(resource)
	span.SetTag("media.type", string(layerManifest.MediaType))
	span.SetTag("layer.digest", layerManifest.Digest.String())
	span.SetTag("layer.size", layerManifest.Size)
	defer func() { span.SetTag("network_attempts", attempts) }()

	attempts, err = withNetworkRetries(
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

			switch layerManifest.MediaType {
			case DatadogPackageLayerMediaType, DatadogPackageConfigLayerMediaType, DatadogPackageExtensionLayerMediaType:
				err = tar.Extract(uncompressedLayer, dir, layerMaxSize)
			case DatadogPackageInstallerLayerMediaType:
				err = writeBinary(uncompressedLayer, dir)
			default:
				return fmt.Errorf("unsupported layer media type: %s", layerManifest.MediaType)
			}
			uncompressedLayer.Close()
			if err != nil {
				return err
			}
			return nil
		},
	)
	return attempts, err
}

// WriteOCILayout writes the image as an OCI layout to the given directory.
func (d *DownloadedPackage) WriteOCILayout(ctx context.Context, dir string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "oci.write_layout")
	defer func() { span.Finish(err) }()
	span.SetTag("package.name", d.Name)
	attempts := 0
	defer func() { span.SetTag("network_attempts", attempts) }()

	var layoutPath layout.Path
	attempts, err = withNetworkRetries(
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
	return err
}

// PackageURL returns the package URL for the given site, package and version.
// Both base and FIPS flavors live under the same URL — flavor selection
// happens at download time via Platform.Variant in downloadIndex.
func PackageURL(env *env.Env, pkg string, version string) string {
	switch env.Site {
	case "datad0g.com":
		return fmt.Sprintf("oci://install.datad0g.com/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	default:
		return fmt.Sprintf("oci://install.datadoghq.com/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
	}
}

// withNetworkRetries calls f and retries it on transient network errors.
// It returns the total number of attempts made (1 if f succeeded on the first try,
// up to networkRetries if all attempts failed) alongside the final error.
func withNetworkRetries(f func() error) (attempts int, err error) {
	for attempts = 1; attempts <= networkRetries; attempts++ {
		err = f()
		if err == nil {
			return attempts, nil
		}
		if !isRetryableNetworkError(err) {
			return attempts, err
		}
		log.Warnf("retrying after network error: %s", err)
		time.Sleep(time.Second)
	}
	return networkRetries, err
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

	if strings.Contains(err.Error(), "connectex") { // Windows
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
