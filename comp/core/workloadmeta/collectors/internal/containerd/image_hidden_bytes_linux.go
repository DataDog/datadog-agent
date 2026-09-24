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
	"path/filepath"
	"slices"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/opencontainers/go-digest"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	hiddenBytesScanTimeout   = time.Minute
	hiddenBytesSweepInterval = time.Second
	hiddenBytesMaxAttempts   = 3
	hiddenBytesRetryDelay    = 30 * time.Second
)

type hiddenLayerResult struct {
	DiffID string `json:"diff_id"`
	Bytes  uint64 `json:"bytes"`
}

type hiddenBytesResult struct {
	Layers           []hiddenLayerResult `json:"layers"`
	UncompressedSize *uint64             `json:"uncompressed_size"`
}

func (r *hiddenBytesResult) clone() *hiddenBytesResult {
	if r == nil {
		return nil
	}
	copy := &hiddenBytesResult{Layers: slices.Clone(r.Layers)}
	if r.UncompressedSize != nil {
		value := *r.UncompressedSize
		copy.UncompressedSize = &value
	}
	return copy
}

func (r *hiddenBytesResult) valid() bool {
	if r == nil || r.UncompressedSize == nil || len(r.Layers) == 0 || len(r.Layers) > hiddenBytesCacheMaxLayers {
		return false
	}
	remaining := *r.UncompressedSize
	for _, layer := range r.Layers {
		if digest.Digest(layer.DiffID).Validate() != nil || layer.Bytes > remaining {
			return false
		}
		remaining -= layer.Bytes
	}
	return true
}

type hiddenBytesScan func(context.Context, *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error)

type hiddenBytesJob struct {
	sequence    uint64
	attempts    int
	nextAttempt time.Time
	finished    bool
}

// Jobs only hold scheduling metadata for currently known images. A single
// worker discovers jobs from this inventory, so a full wake channel cannot drop
// an image and there is no unbounded work queue or per-image goroutine.
type hiddenBytesCollector struct {
	wake     chan struct{}
	done     chan struct{}
	cancel   context.CancelFunc
	scan     hiddenBytesScan
	jobs     map[string]*hiddenBytesJob // guarded by handleImagesMut
	sequence uint64
	cache    *hiddenBytesCache // owned by the worker
}

func (c *collector) initHiddenBytesCollection() {
	if !c.imageMetadataCollectionIsEnabled() || !c.cfg.GetBool("container_image.hidden_bytes.enabled") {
		return
	}
	cache := newHiddenBytesCache(filepath.Join(c.cfg.GetString("run_path"), "container-image-hidden-bytes-v3.json"))
	if err := cache.load(); err != nil {
		log.Debugf("Ignoring unavailable container image hidden-bytes cache: %v", err)
	}
	c.hiddenBytes = &hiddenBytesCollector{
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
		scan:  c.scanImageHiddenBytes,
		jobs:  make(map[string]*hiddenBytesJob),
		cache: cache,
	}
}

func (c *collector) startHiddenBytesCollection(ctx context.Context) {
	if c.hiddenBytes == nil {
		return
	}
	ctx, c.hiddenBytes.cancel = context.WithCancel(ctx)
	go c.runHiddenBytesCollection(ctx)
}

func (c *collector) stopHiddenBytesCollection() {
	if c.hiddenBytes == nil || c.hiddenBytes.cancel == nil {
		return
	}
	c.hiddenBytes.cancel()
	<-c.hiddenBytes.done
}

func (c *collector) enqueueHiddenBytesImageLocked(img *workloadmeta.ContainerImageMetadata) {
	h := c.hiddenBytes
	if h == nil {
		return
	}
	if job, exists := h.jobs[img.ID]; !exists {
		h.sequence++
		h.jobs[img.ID] = &hiddenBytesJob{sequence: h.sequence}
	} else if job.finished && !hasCompleteHiddenBytes(img) {
		// A transient config-read failure can remove layer metadata from a
		// runtime update. Recover the completed result from cache once the
		// metadata is available again instead of permanently losing it.
		job.finished = false
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (c *collector) forgetHiddenBytesImageLocked(id string) {
	if c.hiddenBytes != nil {
		delete(c.hiddenBytes.jobs, id)
	}
}

// A genuine runtime event can signal that a previously unavailable snapshot
// was unpacked. SBOM and hidden-byte result publications never reset retries.
func (c *collector) retryHiddenBytesImageLocked(id string) {
	if c.hiddenBytes == nil {
		return
	}
	job := c.hiddenBytes.jobs[id]
	if job != nil && job.attempts >= hiddenBytesMaxAttempts {
		job.attempts = 0
		// Preserve the last backoff even when runtime events arrive in a burst.
		c.hiddenBytes.sequence++
		job.sequence = c.hiddenBytes.sequence
	}
}

func (c *collector) runHiddenBytesCollection(ctx context.Context) {
	h := c.hiddenBytes
	defer close(h.done)
	ticker := time.NewTicker(hiddenBytesSweepInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if img, job := c.nextHiddenBytesImage(time.Now()); img != nil {
			c.collectHiddenBytes(ctx, img, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-h.wake:
		case <-ticker.C:
		}
	}
}

func (c *collector) nextHiddenBytesImage(now time.Time) (*workloadmeta.ContainerImageMetadata, *hiddenBytesJob) {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()
	var selected *hiddenBytesJob
	var img *workloadmeta.ContainerImageMetadata
	for id, job := range c.hiddenBytes.jobs {
		if job.finished || job.attempts >= hiddenBytesMaxAttempts || now.Before(job.nextAttempt) {
			continue
		}
		current := c.latestImages[id]
		if current == nil {
			continue
		}
		if selected == nil || job.sequence < selected.sequence {
			selected = job
			img = current
		}
	}
	if img == nil {
		return nil, nil
	}
	copy := *img
	copy.Layers = slices.Clone(img.Layers)
	return &copy, selected
}

func (c *collector) collectHiddenBytes(ctx context.Context, img *workloadmeta.ContainerImageMetadata, job *hiddenBytesJob) {
	h := c.hiddenBytes
	results, cached := h.cache.get(img.ID)
	if cached && !hiddenResultsMatch(img, results) {
		cached = false
	}
	if cached {
		log.Debugf("Container image hidden-byte cache hit for %s", img.ID)
	}
	var err error
	if !cached {
		scanCtx, cancel := context.WithTimeout(ctx, hiddenBytesScanTimeout)
		results, err = h.scan(scanCtx, img)
		cancel()
		if err == nil && !hiddenResultsMatch(img, results) {
			err = errors.New("hidden-byte results do not match ordered image layers")
		}
	}
	if ctx.Err() != nil {
		return
	}
	c.handleImagesMut.Lock()
	current := c.latestImages[img.ID]
	// A deletion followed by re-creation has a different job, even if its image
	// ID is unchanged. Never publish a result belonging to the old lifetime.
	if ctx.Err() != nil || current == nil || h.jobs[img.ID] != job {
		c.handleImagesMut.Unlock()
		return
	}
	if err == nil && !hiddenResultsMatch(current, results) {
		err = errors.New("current image metadata no longer matches hidden-byte results")
	}
	if err != nil {
		job.attempts++
		job.nextAttempt = time.Now().Add(hiddenBytesRetryDelay * time.Duration(1<<(job.attempts-1)))
		attempts := job.attempts
		c.handleImagesMut.Unlock()
		log.Debugf("Container image hidden-byte scan unavailable for %s (attempt %d): %v", img.ID, attempts, err)
		return
	}
	updated := *current
	updated.Layers = slices.Clone(current.Layers)
	total := *results.UncompressedSize
	updated.UncompressedSizeBytes = &total
	index := 0
	for i := range updated.Layers {
		if updated.Layers[i].History != nil && updated.Layers[i].History.EmptyLayer {
			continue
		}
		value := results.Layers[index].Bytes
		updated.Layers[i].HiddenBytes = &value
		index++
	}
	job.finished = true
	c.publishImageLocked(&updated)
	c.handleImagesMut.Unlock()
	if !cached {
		log.Debugf("Container image hidden-byte scan completed for %s (%d layers)", img.ID, len(results.Layers))
		if err := h.cache.put(img.ID, results); err != nil {
			log.Debugf("Unable to persist container image hidden-byte result: %v", err)
		}
	}
}

func hiddenResultsMatch(img *workloadmeta.ContainerImageMetadata, results *hiddenBytesResult) bool {
	if !results.valid() {
		return false
	}
	index := 0
	for _, layer := range img.Layers {
		if layer.History != nil && layer.History.EmptyLayer {
			continue
		}
		if layer.DiffID == "" || index >= len(results.Layers) || layer.DiffID != results.Layers[index].DiffID {
			return false
		}
		index++
	}
	return index > 0 && index == len(results.Layers)
}

func (c *collector) scanImageHiddenBytes(ctx context.Context, meta *workloadmeta.ContainerImageMetadata) (*hiddenBytesResult, error) {
	ctx = namespaces.WithNamespace(ctx, meta.Namespace)
	img, err := c.containerdClient.Image(meta.Namespace, meta.Name)
	if err != nil {
		return nil, err
	}
	resolved, err := cutil.ResolveHiddenBytesImage(ctx, img.ContentStore(), img.Target(), img.Platform(), digest.Digest(meta.ID))
	if err != nil {
		return nil, err
	}
	bytes, err := scanHiddenBytesWithFallback(ctx, func(ctx context.Context) (*cutil.HiddenBytesResult, error) {
		layers, cleanup, err := cutil.AcquireImageLayers(ctx, c.containerdClient, meta.Namespace, resolved.Manifest, resolved.DiffIDs, hiddenBytesScanTimeout)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err := cleanup(ctx); err != nil {
				log.Debugf("Unable to clean up hidden-byte image snapshot: %v", err)
			}
		}()
		return cutil.CalculateHiddenBytes(ctx, layers, cutil.DefaultHiddenBytesLimits())
	}, func(ctx context.Context) (*cutil.HiddenBytesResult, error) {
		return cutil.CalculateHiddenBytesFromContent(ctx, img.ContentStore(), resolved, cutil.DefaultHiddenBytesLimits())
	})
	if err != nil {
		return nil, err
	}
	if bytes == nil || len(bytes.Counts) != len(resolved.DiffIDs) {
		return nil, errors.New("hidden-byte result count does not match image layers")
	}
	results := make([]hiddenLayerResult, len(resolved.DiffIDs))
	for i, diffID := range resolved.DiffIDs {
		results[i] = hiddenLayerResult{DiffID: diffID.String(), Bytes: bytes.Counts[i]}
	}
	return &hiddenBytesResult{Layers: results, UncompressedSize: &bytes.UncompressedSize}, nil
}

func scanHiddenBytesWithFallback(ctx context.Context, snapshot, archive func(context.Context) (*cutil.HiddenBytesResult, error)) (*cutil.HiddenBytesResult, error) {
	counts, snapshotErr := snapshot(ctx)
	if snapshotErr == nil {
		log.Debug("Collected container image hidden bytes from overlayfs snapshots")
		return counts, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	counts, archiveErr := archive(ctx)
	if archiveErr != nil {
		return nil, errors.Join(fmt.Errorf("snapshot hidden-byte scan: %w", snapshotErr), fmt.Errorf("local archive hidden-byte scan: %w", archiveErr))
	}
	log.Debug("Collected container image hidden bytes from local layer archives")
	return counts, nil
}
