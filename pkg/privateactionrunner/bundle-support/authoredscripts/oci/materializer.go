// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package oci materializes authored-script Datadog Packages.
package oci

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact"
	artifactoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact/safetar"
	fleetoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

const (
	materializerID         = "authored-script-datadog-package-v1"
	maxPackageLayers       = 32
	maxCompressedLayerSize = 256 << 20
	maxExpandedLayerSize   = 512 << 20
	maxFileSize            = 256 << 20
	maxEntriesPerLayer     = 10_000
)

// Materializer maps a script's main layer and named tool extension layers into
// the layout understood by the authored-script runtime.
type Materializer struct {
	client    *artifactoci.Client
	extractor safetar.Extractor
}

// NewMaterializer creates the authored-script layout policy for an OCI client.
func NewMaterializer(client *artifactoci.Client) (*Materializer, error) {
	if client == nil {
		return nil, errors.New("OCI artifact client is required for authored scripts")
	}
	return &Materializer{
		client: client,
		extractor: safetar.Extractor{Limits: safetar.Limits{
			MaxExpandedBytes: maxExpandedLayerSize,
			MaxFileBytes:     maxFileSize,
			MaxEntries:       maxEntriesPerLayer,
		}},
	}, nil
}

// ID identifies the materialized layout and validation contract.
func (m *Materializer) ID() string { return materializerID }

// Platform returns the host artifact variant selected by the OCI client.
func (m *Materializer) Platform() artifact.Platform {
	if m == nil || m.client == nil {
		return artifact.Platform{}
	}
	return m.client.Platform()
}

// Materialize downloads and extracts the main script and named tool layers.
func (m *Materializer) Materialize(ctx context.Context, descriptor artifact.Descriptor, destination string) error {
	if m == nil || m.client == nil {
		return errors.New("authored-script materializer is not configured")
	}
	pkg, err := m.client.Download(ctx, descriptor)
	if err != nil {
		return err
	}
	layers := pkg.Layers()
	if len(layers) > maxPackageLayers {
		return fmt.Errorf("authored-script package contains %d layers, exceeding the %d layer limit", len(layers), maxPackageLayers)
	}

	var mainLayer *artifactoci.Layer
	extensions := make(map[string]artifactoci.Layer)
	for _, layer := range layers {
		switch layer.MediaType {
		case fleetoci.DatadogPackageLayerMediaType:
			if mainLayer != nil {
				return errors.New("authored-script package contains multiple main layers")
			}
			layerCopy := layer
			mainLayer = &layerCopy
		case fleetoci.DatadogPackageExtensionLayerMediaType:
			name := layer.Annotations[artifactoci.AnnotationExtensionName]
			if !isToolName(name) {
				return fmt.Errorf("authored-script extension name %q must be a single path component", name)
			}
			if _, found := extensions[name]; found {
				return fmt.Errorf("authored-script package contains duplicate extension %q", name)
			}
			extensions[name] = layer
		default:
			return fmt.Errorf("authored-script package contains unsupported layer media type %q", layer.MediaType)
		}
	}
	if mainLayer == nil {
		return errors.New("authored-script package does not contain a main layer")
	}

	if err := pkg.ExtractLayer(ctx, *mainLayer, filepath.Join(destination, "script"), m.extractor, maxCompressedLayerSize); err != nil {
		return err
	}
	toolsDirectory := filepath.Join(destination, "tools")
	if err := os.Mkdir(toolsDirectory, 0o700); err != nil {
		return fmt.Errorf("could not create authored-script tools directory: %w", err)
	}
	for _, name := range slices.Sorted(maps.Keys(extensions)) {
		layer := extensions[name]
		if err := pkg.ExtractLayer(ctx, layer, filepath.Join(toolsDirectory, name), m.extractor, maxCompressedLayerSize); err != nil {
			return fmt.Errorf("could not materialize authored-script tool %q: %w", name, err)
		}
	}
	return makeReadOnly(destination)
}

// Validate checks the authored-script manifest and executable layout.
func (m *Materializer) Validate(ctx context.Context, descriptor artifact.Descriptor, directory string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return authoredscripts.ValidateMaterializedPackage(descriptor, directory)
}

func isToolName(name string) bool {
	return name != "" && name != "." && filepath.IsLocal(name) && !strings.ContainsAny(name, `/\`)
}

func makeReadOnly(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o500)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("authored-script materialization contains unsupported file %q", path)
		}
		mode := os.FileMode(0o400)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o500
		}
		return os.Chmod(path, mode)
	})
}
