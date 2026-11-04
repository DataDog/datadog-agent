// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

const (
	// Register this metric as non-billable in dd-analytics before release.
	cacheSizeMetric      = "datadog.actions.private_runner.authored_scripts.cache.size_bytes"
	cacheMetricsInterval = 5 * time.Minute
)

// StartCacheMetrics periodically reports file bytes in the authored-script cache.
func StartCacheMetrics(ctx context.Context, client statsd.ClientInterface) (func(context.Context) error, error) {
	root, err := defaultArtifactStoreRoot()
	if err != nil {
		return nil, err
	}
	return startCacheMetrics(ctx, root, client), nil
}

func startCacheMetrics(ctx context.Context, root string, client statsd.ClientInterface) func(context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	report := func(ctx context.Context) {
		bytes, err := cacheSize(ctx, root)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.FromContext(ctx).Warn("Could not measure authored-script cache", log.ErrorField(err))
		} else {
			_ = client.Gauge(cacheSizeMetric, float64(bytes), nil, 1)
		}
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(cacheMetricsInterval)
		defer ticker.Stop()
		for {
			report(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	var stopOnce sync.Once
	var stopErr error
	return func(ctx context.Context) error {
		stopOnce.Do(func() {
			cancel()
			select {
			case <-done:
				report(ctx)
			case <-ctx.Done():
				stopErr = ctx.Err()
			}
		})
		return stopErr
	}
}

// cacheSize sums regular-file lengths beneath root, including staging files.
func cacheSize(ctx context.Context, root string) (int64, error) {
	var bytes int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		bytes += info.Size()
		return nil
	})
	return bytes, err
}
