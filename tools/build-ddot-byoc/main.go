// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

const (
	extensionAnnotationKey = "com.datadoghq.package.extension.name"
	ddotExtensionName      = "ddot"

	agentPackageMediaType  types.MediaType = "application/vnd.datadog.package.v1"
	ddotExtensionMediaType types.MediaType = "application/vnd.datadog.package.extension.layer.v1.tar+zstd"
)

func main() {
	agentOCI := flag.String("agent-oci", "", "OCI URL for the source Datadog Agent package (required)")
	otelAgent := flag.String("otel-agent", "", "Path to the custom otel-agent binary (required)")
	outputOCI := flag.String("output-oci", "", "OCI URL to push the customized package to (required)")
	targetOS := flag.String("os", runtime.GOOS, "Target OS of the agent package")
	targetArch := flag.String("arch", runtime.GOARCH, "Target architecture of the agent package")
	flag.Parse()

	if *agentOCI == "" || *otelAgent == "" || *outputOCI == "" {
		fmt.Fprintln(os.Stderr, "usage: build-ddot-byoc --agent-oci <url> --otel-agent <path> --output-oci <url>")
		fmt.Fprintln(os.Stderr, "\nflags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if err := run(*agentOCI, *otelAgent, *outputOCI, *targetOS, *targetArch); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(agentOCI, otelAgentPath, outputOCI, targetOS, targetArch string) error {
	ctx := context.Background()
	keychain := authn.DefaultKeychain

	// Parse source reference (strip oci:// prefix if present).
	srcRef, err := name.ParseReference(strings.TrimPrefix(agentOCI, "oci://"))
	if err != nil {
		return fmt.Errorf("parsing source reference: %w", err)
	}

	fmt.Printf("Pulling source OCI image from %s\n", agentOCI)
	srcIndex, err := remote.Index(srcRef, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("pulling source index: %w", err)
	}

	indexManifest, err := srcIndex.IndexManifest()
	if err != nil {
		return fmt.Errorf("loading index manifest: %w", err)
	}

	// Find the manifest matching the requested platform.
	platform := v1.Platform{OS: targetOS, Architecture: targetArch}
	var selectedDesc *v1.Descriptor
	for i, desc := range indexManifest.Manifests {
		if desc.Platform == nil || desc.Platform.Satisfies(platform) {
			selectedDesc = &indexManifest.Manifests[i]
			break
		}
	}
	if selectedDesc == nil {
		return fmt.Errorf("no manifest found for platform %s/%s", targetOS, targetArch)
	}
	fmt.Printf("Found manifest for %s/%s: %s\n", targetOS, targetArch, selectedDesc.Digest)

	srcImage, err := srcIndex.Image(selectedDesc.Digest)
	if err != nil {
		return fmt.Errorf("loading image for platform %s/%s: %w", targetOS, targetArch, err)
	}

	fmt.Printf("Reading custom otel-agent from %s\n", otelAgentPath)
	newBinaryData, err := os.ReadFile(otelAgentPath)
	if err != nil {
		return fmt.Errorf("reading otel-agent binary: %w", err)
	}

	// The binary inside the extension layer lives at embedded/bin/otel-agent[.exe].
	binaryName := "otel-agent"
	if targetOS == "windows" {
		binaryName = "otel-agent.exe"
	}
	binaryPath := "embedded/bin/" + binaryName

	fmt.Printf("Replacing %s in ddot extension layer...\n", binaryPath)
	newImage, err := replaceDdotBinary(srcImage, newBinaryData, binaryPath)
	if err != nil {
		return fmt.Errorf("replacing ddot binary: %w", err)
	}

	// Build a single-platform OCI index containing the modified image.
	newIndex := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
		Add: newImage,
		Descriptor: v1.Descriptor{
			Platform: selectedDesc.Platform,
		},
	})
	if len(indexManifest.Annotations) > 0 {
		newIndex = mutate.Annotations(newIndex, indexManifest.Annotations).(v1.ImageIndex)
	}

	dstRef, err := name.ParseReference(strings.TrimPrefix(outputOCI, "oci://"))
	if err != nil {
		return fmt.Errorf("parsing output reference: %w", err)
	}

	fmt.Printf("Pushing customized package to %s\n", outputOCI)
	if err := remote.WriteIndex(dstRef, newIndex, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx)); err != nil {
		return fmt.Errorf("pushing to output registry: %w", err)
	}

	digest, err := newIndex.Digest()
	if err != nil {
		return fmt.Errorf("computing digest: %w", err)
	}
	fmt.Printf("Successfully pushed: %s@%s\n", dstRef, digest)
	return nil
}

// replaceDdotBinary rebuilds img with the ddot extension layer replaced by one
// containing newBinaryData at binaryPath.
func replaceDdotBinary(img v1.Image, newBinaryData []byte, binaryPath string) (v1.Image, error) {
	manifest, err := img.Manifest()
	if err != nil {
		return nil, fmt.Errorf("getting manifest: %w", err)
	}

	newImg := empty.Image
	newImg = mutate.MediaType(newImg, types.OCIManifestSchema1)
	newImg = mutate.ConfigMediaType(newImg, agentPackageMediaType)
	if len(manifest.Annotations) > 0 {
		newImg = mutate.Annotations(newImg, manifest.Annotations).(v1.Image)
	}

	var addenda []mutate.Addendum
	ddotFound := false
	for _, layerDesc := range manifest.Layers {
		layer, err := img.LayerByDigest(layerDesc.Digest)
		if err != nil {
			return nil, fmt.Errorf("loading layer %s: %w", layerDesc.Digest, err)
		}

		isTarget := layerDesc.MediaType == ddotExtensionMediaType &&
			layerDesc.Annotations[extensionAnnotationKey] == ddotExtensionName

		if isTarget {
			fmt.Println("  Found ddot extension layer, repacking with new binary...")
			newLayer, err := repackLayer(layer, binaryPath, newBinaryData)
			if err != nil {
				return nil, fmt.Errorf("repacking ddot extension layer: %w", err)
			}
			addenda = append(addenda, mutate.Addendum{
				Layer:       newLayer,
				Annotations: layerDesc.Annotations,
			})
			ddotFound = true
		} else {
			addenda = append(addenda, mutate.Addendum{
				Layer:       layer,
				Annotations: layerDesc.Annotations,
			})
		}
	}

	if !ddotFound {
		return nil, fmt.Errorf("ddot extension layer not found in image")
	}

	newImg, err = mutate.Append(newImg, addenda...)
	if err != nil {
		return nil, fmt.Errorf("building modified image: %w", err)
	}
	return newImg, nil
}

// repackLayer replaces binaryPath inside a zstd-compressed tar layer with
// newBinaryData and returns a new layer with the same media type.
func repackLayer(layer v1.Layer, binaryPath string, newBinaryData []byte) (v1.Layer, error) {
	// layer.Uncompressed() detects and strips zstd compression, returning a raw tar stream.
	uncompressed, err := layer.Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("decompressing layer: %w", err)
	}
	defer uncompressed.Close()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	tr := tar.NewReader(uncompressed)
	replaced := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar entry: %w", err)
		}

		// Handle both "path" and "./path" forms.
		entryName := strings.TrimPrefix(hdr.Name, "./")
		if entryName == binaryPath {
			newHdr := *hdr
			newHdr.Size = int64(len(newBinaryData))
			if err := tw.WriteHeader(&newHdr); err != nil {
				return nil, fmt.Errorf("writing tar header: %w", err)
			}
			if _, err := tw.Write(newBinaryData); err != nil {
				return nil, fmt.Errorf("writing binary data: %w", err)
			}
			// Drain the original entry before moving to the next header.
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, fmt.Errorf("draining original entry: %w", err)
			}
			replaced = true
		} else {
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, fmt.Errorf("writing tar header: %w", err)
			}
			if _, err := io.Copy(tw, tr); err != nil {
				return nil, fmt.Errorf("copying tar entry: %w", err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar writer: %w", err)
	}
	if !replaced {
		return nil, fmt.Errorf("binary path %q not found in ddot extension layer", binaryPath)
	}

	// Recompress with zstd.
	var zstdBuf bytes.Buffer
	zw, err := zstd.NewWriter(&zstdBuf)
	if err != nil {
		return nil, fmt.Errorf("creating zstd encoder: %w", err)
	}
	if _, err := io.Copy(zw, &tarBuf); err != nil {
		return nil, fmt.Errorf("encoding zstd: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("closing zstd encoder: %w", err)
	}

	return static.NewLayer(zstdBuf.Bytes(), ddotExtensionMediaType), nil
}
