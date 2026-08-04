// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	ociTypes "github.com/google/go-containerregistry/pkg/v1/types"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

const (
	catalogScriptPrefix = "dd-par-scripts:"
	catalogRegistry     = "us-docker.pkg.dev/datadog-sandbox/par-scripts"

	ddPackageLayerMediaType    ociTypes.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	ddPackageExtLayerMediaType ociTypes.MediaType = "application/vnd.datadog.package.extension.layer.v1.tar+zstd"
	ddPackageExtNameAnnotation                    = "com.datadoghq.package.extension.name"
)

func isCatalogScript(scriptName string) bool {
	return strings.HasPrefix(scriptName, catalogScriptPrefix)
}

// catalogScriptResult holds what resolveCatalogScript returns.
type catalogScriptResult struct {
	config              RunPredefinedScriptConfig
	toolDirs            []string
	envVars             map[string]string
	parameterEnvMapping map[string]string
	cleanup             func()
}

// resolveCatalogScript downloads the OCI package for a dd-par-scripts: entry,
// extracts the script and its tool binaries, reads metadata.json, and returns
// a fully populated RunPredefinedScriptConfig plus tool binary dirs and a
// cleanup function to remove the extracted files after execution.
// catalogCacheDir returns the persistent directory for a specific package version
// and platform. Keying by version means the content is immutable and safe to
// reuse across runs without re-validating against the registry.
func catalogCacheDir(pkgName, version, goos, arch string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user cache dir: %w", err)
	}
	return filepath.Join(base, "datadog-par-scripts", pkgName, version, goos, arch), nil
}

func resolveCatalogScript(ctx context.Context, scriptName string, version string) (*catalogScriptResult, error) {
	pkgName := strings.TrimPrefix(scriptName, catalogScriptPrefix)
	if version == "" {
		return nil, fmt.Errorf("catalog script %q requires a version (set via 'version:' in script.yaml)", scriptName)
	}

	goos := runtime.GOOS
	arch := runtime.GOARCH
	logger := log.FromContext(ctx)

	installDir, err := catalogCacheDir(pkgName, version, goos, arch)
	if err != nil {
		return nil, err
	}

	// Cache hit: the directory already contains an extracted package.
	// We key by (pkgName, version, os, arch) so the content is immutable.
	if _, err := os.Stat(filepath.Join(installDir, "scripts", "metadata.json")); err == nil {
		logger.Debugf("catalog: cache hit for %s:%s (%s/%s) at %s", pkgName, version, goos, arch, installDir)
	} else {
		imageRef := fmt.Sprintf("%s/%s:%s", catalogRegistry, pkgName, version)
		logger.Debugf("catalog: cache miss — pulling %s (platform=%s/%s)", imageRef, goos, arch)

		if err := downloadCatalogPackage(ctx, logger, imageRef, goos, arch, installDir); err != nil {
			os.RemoveAll(installDir) // remove partial extraction
			return nil, err
		}
		logger.Debugf("catalog: package extracted to cache at %s", installDir)
	}

	return buildCatalogResult(logger, scriptName, version, installDir)
}

// downloadCatalogPackage pulls an OCI image and extracts its layers into installDir.
func downloadCatalogPackage(ctx context.Context, logger log.Logger, imageRef, goos, arch, installDir string) error {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return fmt.Errorf("invalid catalog package reference %s: %w", imageRef, err)
	}

	img, err := remote.Image(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithPlatform(v1.Platform{OS: goos, Architecture: arch}),
	)
	if err != nil {
		return fmt.Errorf("could not pull %s: %w", imageRef, err)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("could not create install dir: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("could not get image manifest: %w", err)
	}
	logger.Debugf("catalog: manifest has %d layers", len(manifest.Layers))

	for i, layerDesc := range manifest.Layers {
		logger.Debugf("catalog: layer[%d] mediaType=%s digest=%s size=%d", i, layerDesc.MediaType, layerDesc.Digest, layerDesc.Size)

		layer, err := img.LayerByDigest(layerDesc.Digest)
		if err != nil {
			return fmt.Errorf("could not get layer %s: %w", layerDesc.Digest, err)
		}

		switch layerDesc.MediaType {
		case ddPackageLayerMediaType:
			logger.Debugf("catalog: extracting main layer to %s", installDir)
			r, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not decompress main layer: %w", err)
			}
			err = extractTarInto(r, installDir)
			r.Close()
			if err != nil {
				return fmt.Errorf("could not extract main layer: %w", err)
			}
			logger.Debugf("catalog: main layer extracted")

		case ddPackageExtLayerMediaType:
			extName := layerDesc.Annotations[ddPackageExtNameAnnotation]
			if extName == "" {
				logger.Debugf("catalog: skipping extension layer with no name annotation")
				continue
			}
			toolDir := filepath.Join(installDir, "tools", extName)
			logger.Debugf("catalog: extracting extension layer %q to %s", extName, toolDir)
			if err := os.MkdirAll(toolDir, 0755); err != nil {
				return err
			}
			r, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not decompress extension layer %s: %w", extName, err)
			}
			err = extractTarInto(r, toolDir)
			r.Close()
			if err != nil {
				return fmt.Errorf("could not extract extension layer %s: %w", extName, err)
			}
			logger.Debugf("catalog: extension layer %q extracted", extName)

		default:
			logger.Debugf("catalog: skipping unknown layer mediaType=%s", layerDesc.MediaType)
		}
	}
	return nil
}

// buildCatalogResult reads metadata from an already-extracted installDir and
// assembles the catalogScriptResult. Called for both cache hits and misses.
func buildCatalogResult(logger log.Logger, scriptName, version, installDir string) (*catalogScriptResult, error) {
	meta, err := readScriptMetadata(installDir)
	if err != nil {
		return nil, err
	}
	logger.Debugf("catalog: metadata loaded — command=%v allowedEnvVars=%v parameterEnvMapping=%v", meta.Command, meta.AllowedEnvVars, meta.ParameterEnvMapping)

	cmd := make([]string, len(meta.Command))
	copy(cmd, meta.Command)
	cmd[0] = filepath.Join(installDir, "scripts", meta.Command[0])

	// Collect tool dirs from the extracted cache.
	var toolDirs []string
	toolsRoot := filepath.Join(installDir, "tools")
	if entries, err := os.ReadDir(toolsRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				toolDirs = append(toolDirs, filepath.Join(toolsRoot, e.Name()))
			}
		}
	}

	logger.Debugf("catalog: resolved command: %v", cmd)
	logger.Debugf("catalog: tool dirs in PATH: %v", toolDirs)

	return &catalogScriptResult{
		config: RunPredefinedScriptConfig{
			Command:         cmd,
			ParameterSchema: meta.ParameterSchema,
			AllowedEnvVars:  meta.AllowedEnvVars,
			Version:         version,
		},
		toolDirs:            toolDirs,
		envVars:             meta.EnvVars,
		parameterEnvMapping: meta.ParameterEnvMapping,
		cleanup:             func() {}, // cached installs are never deleted
	}, nil
}

// extractTarInto writes a tar stream into destDir, creating files and directories.
func extractTarInto(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}
		if header.Name == "./" || header.Name == "." {
			continue
		}

		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %q would escape destination directory", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("could not create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("could not create file %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("could not write file %s: %w", target, err)
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("could not create symlink %s: %w", target, err)
			}
		}
	}
	return nil
}
