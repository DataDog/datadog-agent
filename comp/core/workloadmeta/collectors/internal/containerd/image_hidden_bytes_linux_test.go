// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/platforms"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

type hiddenBytesTestStore struct {
	workloadmeta.Component
	mu     sync.Mutex
	events []workloadmeta.CollectorEvent
}

func (s *hiddenBytesTestStore) Notify(events []workloadmeta.CollectorEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
}

func newHiddenBytesTestCollector(scan hiddenBytesScan) (*collector, *hiddenBytesTestStore) {
	store := &hiddenBytesTestStore{}
	return &collector{
		store: store,
		hiddenBytes: &hiddenBytesCollector{
			wake:  make(chan struct{}, 1),
			done:  make(chan struct{}),
			scan:  scan,
			jobs:  make(map[string]*hiddenBytesJob),
			cache: newHiddenBytesCache(""),
		},
	}, store
}

func hiddenBytesTestImage(name string) *workloadmeta.ContainerImageMetadata {
	return &workloadmeta.ContainerImageMetadata{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: digest.FromString(name).String()},
		EntityMeta: workloadmeta.EntityMeta{Name: name, Namespace: "k8s.io"},
		Layers: []workloadmeta.ContainerImageLayer{
			{DiffID: digest.FromString("base").String()},
			{History: &ocispec.History{EmptyLayer: true}},
			{DiffID: digest.FromString("top").String()},
		},
	}
}

func hiddenBytesTestResult(img *workloadmeta.ContainerImageMetadata) *hiddenBytesResult {
	return &hiddenBytesResult{UncompressedSize: pointer.Ptr(uint64(100)), Layers: []hiddenLayerResult{
		{DiffID: img.Layers[0].DiffID, Bytes: 10},
		{DiffID: img.Layers[2].DiffID, Bytes: 0},
	}}
}

func publishHiddenBytesTestImage(c *collector, img *workloadmeta.ContainerImageMetadata) {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()
	c.publishImageLocked(img)
}

func TestHiddenBytesCoalescingAndSaturatedWake(t *testing.T) {
	scans := 0
	c, store := newHiddenBytesTestCollector(func(_ context.Context, img *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
		scans++
		return hiddenBytesTestResult(img), nil
	})
	for i := 0; i < 4; i++ {
		img := hiddenBytesTestImage(fmt.Sprintf("image-%d", i))
		publishHiddenBytesTestImage(c, img)
		publishHiddenBytesTestImage(c, img)
	}
	require.Len(t, c.hiddenBytes.wake, 1)
	require.Len(t, c.hiddenBytes.jobs, 4)
	for i := 0; i < 4; i++ {
		img, job := c.nextHiddenBytesImage(time.Now())
		require.NotNil(t, img)
		c.collectHiddenBytes(t.Context(), img, job)
		updated := c.latestImages[img.ID]
		assert.Equal(t, pointer.Ptr(uint64(100)), updated.UncompressedSizeBytes)
		require.NotNil(t, updated.Layers[0].HiddenBytes)
		assert.Equal(t, uint64(10), *updated.Layers[0].HiddenBytes)
		assert.Nil(t, updated.Layers[1].HiddenBytes)
		require.NotNil(t, updated.Layers[2].HiddenBytes)
		assert.Zero(t, *updated.Layers[2].HiddenBytes)
	}
	img, _ := c.nextHiddenBytesImage(time.Now())
	assert.Nil(t, img)
	assert.Equal(t, 4, scans)
	assert.Len(t, store.events, 12)
}

func TestHiddenBytesPreservesConcurrentMetadata(t *testing.T) {
	img := hiddenBytesTestImage("updated")
	var c *collector
	c, _ = newHiddenBytesTestCollector(func(_ context.Context, _ *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
		updated := *img
		updated.RepoTags = []string{"new-tag"}
		updated.SBOM = &workloadmeta.CompressedSBOM{Status: workloadmeta.Success, GenerationMethod: "overlayfs"}
		publishHiddenBytesTestImage(c, &updated)
		return hiddenBytesTestResult(img), nil
	})
	publishHiddenBytesTestImage(c, img)
	request, job := c.nextHiddenBytesImage(time.Now())
	c.collectHiddenBytes(t.Context(), request, job)
	updated := c.latestImages[img.ID]
	assert.Equal(t, []string{"new-tag"}, updated.RepoTags)
	require.NotNil(t, updated.SBOM)
	assert.Equal(t, workloadmeta.Success, updated.SBOM.Status)
	assert.Nil(t, img.Layers[0].HiddenBytes, "previously published entity must remain immutable")
	assert.NotNil(t, updated.Layers[0].HiddenBytes)
}

func TestHiddenBytesRecoversAfterMetadataLoss(t *testing.T) {
	for _, afterCompletion := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-completion=%t", afterCompletion), func(t *testing.T) {
			img := hiddenBytesTestImage("metadata-recovery")
			var c *collector
			scans := 0
			c, _ = newHiddenBytesTestCollector(func(_ context.Context, _ *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
				scans++
				if !afterCompletion && scans == 1 {
					incomplete := *img
					incomplete.Layers = nil
					publishHiddenBytesTestImage(c, &incomplete)
				}
				return hiddenBytesTestResult(img), nil
			})
			publishHiddenBytesTestImage(c, img)
			request, job := c.nextHiddenBytesImage(time.Now())
			c.collectHiddenBytes(t.Context(), request, job)
			if afterCompletion {
				incomplete := *img
				incomplete.Layers = nil
				publishHiddenBytesTestImage(c, &incomplete)
			}
			assert.False(t, c.hiddenBytes.jobs[img.ID].finished)
			publishHiddenBytesTestImage(c, img)
			request, job = c.nextHiddenBytesImage(time.Now().Add(time.Hour))
			require.NotNil(t, request)
			c.collectHiddenBytes(t.Context(), request, job)
			require.NotNil(t, c.latestImages[img.ID].Layers[0].HiddenBytes)
			assert.Equal(t, uint64(10), *c.latestImages[img.ID].Layers[0].HiddenBytes)
			if afterCompletion {
				assert.Equal(t, 1, scans, "recover completed results from cache")
			} else {
				assert.Equal(t, 2, scans)
			}
		})
	}
}

func TestHiddenBytesDeleteDuringScan(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("recreate=%t", recreate), func(t *testing.T) {
			img := hiddenBytesTestImage("deleted")
			var c *collector
			var store *hiddenBytesTestStore
			c, store = newHiddenBytesTestCollector(func(_ context.Context, _ *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
				c.handleImagesMut.Lock()
				delete(c.latestImages, img.ID)
				c.forgetHiddenBytesImageLocked(img.ID)
				c.handleImagesMut.Unlock()
				if recreate {
					publishHiddenBytesTestImage(c, img)
				}
				return hiddenBytesTestResult(img), nil
			})
			publishHiddenBytesTestImage(c, img)
			request, job := c.nextHiddenBytesImage(time.Now())
			c.collectHiddenBytes(t.Context(), request, job)
			if recreate {
				assert.Len(t, store.events, 2)
				assert.Nil(t, c.latestImages[img.ID].Layers[0].HiddenBytes)
				assert.NotSame(t, job, c.hiddenBytes.jobs[img.ID])
			} else {
				assert.Len(t, store.events, 1)
				assert.Empty(t, c.latestImages)
				assert.Empty(t, c.hiddenBytes.jobs)
			}
		})
	}
}

func TestHiddenBytesRetryBounds(t *testing.T) {
	c, store := newHiddenBytesTestCollector(func(context.Context, *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
		return nil, errors.New("snapshots not available")
	})
	img := hiddenBytesTestImage("retry")
	publishHiddenBytesTestImage(c, img)
	for i := 0; i < hiddenBytesMaxAttempts; i++ {
		request, job := c.nextHiddenBytesImage(time.Now().Add(time.Hour))
		require.NotNil(t, request)
		c.collectHiddenBytes(t.Context(), request, job)
		immediate, _ := c.nextHiddenBytesImage(time.Now())
		assert.Nil(t, immediate, "retry must respect backoff")
	}
	request, _ := c.nextHiddenBytesImage(time.Now().Add(time.Hour))
	assert.Nil(t, request)
	assert.Len(t, store.events, 1, "failures must not publish zero-byte results")
	assert.Empty(t, c.hiddenBytes.cache.entries)
	// Self-publication/SBOM update does not reset exhausted retries.
	publishHiddenBytesTestImage(c, img)
	assert.Equal(t, hiddenBytesMaxAttempts, c.hiddenBytes.jobs[img.ID].attempts)
	c.handleImagesMut.Lock()
	c.retryHiddenBytesImageLocked(img.ID)
	c.handleImagesMut.Unlock()
	request, _ = c.nextHiddenBytesImage(time.Now())
	assert.Nil(t, request, "a runtime event retains the last backoff")
	request, _ = c.nextHiddenBytesImage(time.Now().Add(time.Hour))
	assert.NotNil(t, request)
}

func TestHiddenBytesCacheAvoidsScan(t *testing.T) {
	c, _ := newHiddenBytesTestCollector(func(context.Context, *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
		t.Fatal("cached image should not be scanned")
		return nil, nil
	})
	img := hiddenBytesTestImage("cached")
	require.NoError(t, c.hiddenBytes.cache.put(img.ID, hiddenBytesTestResult(img)))
	publishHiddenBytesTestImage(c, img)
	request, job := c.nextHiddenBytesImage(time.Now())
	c.collectHiddenBytes(t.Context(), request, job)
	require.NotNil(t, c.latestImages[img.ID].Layers[0].HiddenBytes)
	assert.Equal(t, uint64(10), *c.latestImages[img.ID].Layers[0].HiddenBytes)
}

func TestHiddenBytesOrderedResults(t *testing.T) {
	img := hiddenBytesTestImage("repeated")
	img.Layers[2].DiffID = img.Layers[0].DiffID
	results := hiddenBytesTestResult(img)
	assert.True(t, hiddenResultsMatch(img, results))
	c, _ := newHiddenBytesTestCollector(func(context.Context, *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
		return results, nil
	})
	publishHiddenBytesTestImage(c, img)
	request, job := c.nextHiddenBytesImage(time.Now())
	c.collectHiddenBytes(t.Context(), request, job)
	updated := c.latestImages[img.ID]
	assert.Equal(t, uint64(10), *updated.Layers[0].HiddenBytes)
	assert.Zero(t, *updated.Layers[2].HiddenBytes)
	incomplete := results.clone()
	incomplete.Layers = incomplete.Layers[:1]
	assert.False(t, hiddenResultsMatch(img, incomplete))
	img.Layers[0].DiffID = ""
	assert.False(t, hiddenResultsMatch(img, results))
}

func TestHiddenBytesShutdownCancelsScan(t *testing.T) {
	started := make(chan struct{})
	c, store := newHiddenBytesTestCollector(func(ctx context.Context, _ *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	publishHiddenBytesTestImage(c, hiddenBytesTestImage("shutdown"))
	c.startHiddenBytesCollection(t.Context())
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	c.stopHiddenBytesCollection()
	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.events, 1)
	assert.Empty(t, c.hiddenBytes.cache.entries)
}

func TestHiddenBytesConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name          string
		imagesEnabled bool
		disableHidden bool
		wantEnabled   bool
	}{
		{"enabled by default", true, false, true},
		{"explicitly disabled", true, true, false},
		{"image collection disabled", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			overrides := map[string]interface{}{
				"container_image.enabled": tc.imagesEnabled,
				"run_path":                t.TempDir(),
			}
			if tc.disableHidden {
				overrides["container_image.hidden_bytes.enabled"] = false
			}
			c := &collector{cfg: config.NewMockWithOverrides(t, overrides), store: &hiddenBytesTestStore{}}
			c.initHiddenBytesCollection()
			if tc.wantEnabled {
				require.NotNil(t, c.hiddenBytes)
				return
			}
			c.startHiddenBytesCollection(t.Context())
			defer c.stopHiddenBytesCollection()
			publishHiddenBytesTestImage(c, hiddenBytesTestImage("disabled"))
			assert.Nil(t, c.hiddenBytes)
			assert.Nil(t, c.latestImages)
		})
	}
}

func TestPreserveHiddenBytesRequiresMatchingOrderedLayers(t *testing.T) {
	src := hiddenBytesTestImage("image")
	value := uint64(42)
	src.Layers[0].HiddenBytes = &value
	src.Layers[2].HiddenBytes = pointer.Ptr(uint64(0))
	src.UncompressedSizeBytes = pointer.Ptr(uint64(100))
	dst := hiddenBytesTestImage("image")
	preserveHiddenBytes(dst, src)
	require.NotNil(t, dst.Layers[0].HiddenBytes)
	assert.Equal(t, value, *dst.Layers[0].HiddenBytes)
	assert.Equal(t, src.UncompressedSizeBytes, dst.UncompressedSizeBytes)
	dst = hiddenBytesTestImage("image")
	dst.Layers[2].DiffID = digest.FromString("different").String()
	preserveHiddenBytes(dst, src)
	assert.Nil(t, dst.Layers[0].HiddenBytes, "must not attach partial results to a mismatched stack")
	assert.Nil(t, dst.UncompressedSizeBytes)
}

type hiddenBytesConfigImage struct {
	containerd.Image
	target ocispec.Descriptor
}

func (i hiddenBytesConfigImage) Target() ocispec.Descriptor {
	return i.target
}

func (hiddenBytesConfigImage) ContentStore() content.Store {
	return nil // Inline metadata must not read a blob or reach either scanner.
}

func (hiddenBytesConfigImage) Platform() platforms.MatchComparer {
	return platforms.Default()
}

func TestHiddenBytesRejectsMovedTag(t *testing.T) {
	meta := hiddenBytesTestImage("image:latest")
	c := &collector{containerdClient: &fake.MockedContainerdClient{
		MockImage: func(namespace, name string) (containerd.Image, error) {
			assert.Equal(t, meta.Namespace, namespace)
			assert.Equal(t, meta.Name, name)
			data := []byte(fmt.Sprintf(`{"schemaVersion":2,"config":{"digest":%q}}`, digest.FromString("new-image")))
			return hiddenBytesConfigImage{target: ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromBytes(data), Size: int64(len(data)), Data: data}}, nil
		},
	}}
	result, err := c.scanImageHiddenBytes(t.Context(), meta)
	require.ErrorContains(t, err, "no longer points to")
	assert.Nil(t, result)
}

func TestHiddenBytesRejectsOversizedMetadataBeforeScanning(t *testing.T) {
	meta := hiddenBytesTestImage("oversized")
	c := &collector{containerdClient: &fake.MockedContainerdClient{
		MockImage: func(string, string) (containerd.Image, error) {
			return hiddenBytesConfigImage{target: ocispec.Descriptor{MediaType: ocispec.MediaTypeImageIndex, Digest: digest.FromString("large"), Size: 5 << 20}}, nil
		},
	}}
	result, err := c.scanImageHiddenBytes(t.Context(), meta)
	require.ErrorContains(t, err, "metadata blob size limit")
	assert.Nil(t, result)
}

func TestHiddenBytesScanFallback(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		snapshotErr, archiveErr error
		cancel                  bool
		wantArchive             bool
		want                    *cutil.HiddenBytesResult
	}{
		{name: "snapshot success", want: &cutil.HiddenBytesResult{Counts: []uint64{1, 2}, UncompressedSize: 30}},
		{name: "archive fallback", snapshotErr: errors.New("no snapshots"), wantArchive: true, want: &cutil.HiddenBytesResult{Counts: []uint64{3, 4}, UncompressedSize: 70}},
		{name: "both fail", snapshotErr: errors.New("no snapshots"), archiveErr: errors.New("missing blob"), wantArchive: true},
		{name: "cancelled snapshot", snapshotErr: context.Canceled, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			archiveCalled := false
			got, err := scanHiddenBytesWithFallback(ctx, func(context.Context) (*cutil.HiddenBytesResult, error) {
				if tc.cancel {
					cancel()
				}
				return &cutil.HiddenBytesResult{Counts: []uint64{1, 2}, UncompressedSize: 30}, tc.snapshotErr
			}, func(context.Context) (*cutil.HiddenBytesResult, error) {
				archiveCalled = true
				return &cutil.HiddenBytesResult{Counts: []uint64{3, 4}, UncompressedSize: 70}, tc.archiveErr
			})
			if tc.want == nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantArchive, archiveCalled)
		})
	}
}
