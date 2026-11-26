// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && trivy

package containerd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"testing"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

type missingContentStore struct {
	content.Store
}

func (missingContentStore) ReaderAt(_ context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	return nil, fmt.Errorf("content digest %s: %w", desc.Digest, errdefs.ErrNotFound)
}

type emptyReport struct{}

func (emptyReport) ToCycloneDX() *cyclonedx_v1_4.Bom { return &cyclonedx_v1_4.Bom{} }

func (emptyReport) ID() string { return "" }

func TestProcessScanResultLogLevel(t *testing.T) {
	const namespace = "k8s.io"
	const listed = "registry.example/app:1"

	cl, err := containerd.New("", containerd.WithServices(containerd.WithContentStore(missingContentStore{})))
	require.NoError(t, err)
	target := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Size:      1,
	}
	client := &fake.MockedContainerdClient{
		MockImage: func(_, name string) (containerd.Image, error) {
			if name != listed {
				return nil, fmt.Errorf("image %q: %w", name, errdefs.ErrNotFound)
			}
			return containerd.NewImage(cl, images.Image{Name: name, Target: target}), nil
		},
	}

	provider, err := NewCollector(dependencies{Config: config.NewMock(t), FilterStore: workloadfilterfxmock.SetupMockFilter(t)})
	require.NoError(t, err)
	c := provider.Collector.(*collector)
	c.containerdClient = client

	manifestErr := c.handleImageCreateOrUpdate(context.Background(), namespace, listed, nil)
	require.True(t, errdefs.IsNotFound(manifestErr), "manifest read error: %v", manifestErr)

	tests := []struct {
		name      string
		imageName string
		level     string
		message   string
	}{
		{
			name:      "image removed from containerd",
			imageName: "registry.example/app:removed",
			level:     "[DEBUG]",
			message:   "Dropping SBOM for image removed from containerd",
		},
		{
			name:      "manifest missing for a listed image",
			imageName: listed,
			level:     "[WARN]",
			message:   "Error extracting SBOM for image",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := pkglog.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, pkglog.DebugLvl)
			require.NoError(t, err)
			pkglog.SetupLogger(l, "debug")
			t.Cleanup(func() { pkglog.SetupLogger(pkglog.Default(), "info") })

			c.processScanResult(context.Background(), sbom.ScanResult{
				ImgMeta: &workloadmeta.ContainerImageMetadata{
					EntityMeta: workloadmeta.EntityMeta{Name: tt.imageName, Namespace: namespace},
				},
				Report: emptyReport{},
			})

			l.Close()
			w.Flush()
			logs := b.String()
			assert.Contains(t, logs, tt.level+" "+tt.message, logs)
			assert.Equal(t, 1, bytes.Count(b.Bytes(), []byte("SBOM for image")), logs)
		})
	}
}
