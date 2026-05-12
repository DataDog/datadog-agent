// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"context"
	"fmt"
	"iter"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

// PackageEncoder is the minimal interface RunUploadLoop needs.
type PackageEncoder interface {
	// AddPackage encodes pkg as a "package" scope into the current batch.
	AddPackage(pkg symdb.Package, agentVersion string) error
	// Size reports the current compressed buffer size, used to decide when
	// to flush. Lower bound only — the gzip writer buffers internally.
	Size() int
	// Flush finalises the current batch and ships it. final is set on the
	// last batch of an upload.
	Flush(ctx context.Context, final bool) error
}

// UploadStats summarises a completed RunUploadLoop run.
type UploadStats struct {
	// Packages is the number of packages drained from the iterator.
	Packages int
	// Functions is the cumulative number of functions across those packages
	// (including methods on types).
	Functions int
	// Batches is the number of HTTP batches that the encoder shipped.
	Batches int
}

// RunUploadLoop drains it through enc, flushing whenever Size crosses
// flushThreshold or on the iterator's final yield. Errors from Flush are
// returned unwrapped so callers can errors.Is them against ErrUpload.
//
// RunUploadLoop does not call enc.Close; the caller owns the encoder's
// lifecycle.
func RunUploadLoop(
	ctx context.Context,
	enc PackageEncoder,
	it iter.Seq2[symdb.PackageWithFinal, error],
	agentVersion string,
	flushThreshold int,
) (UploadStats, error) {
	var stats UploadStats
	maybeFlush := func(final bool) error {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if !final && enc.Size() < flushThreshold {
			return nil
		}
		if err := enc.Flush(ctx, final); err != nil {
			return err
		}
		stats.Batches++
		return nil
	}
	for pkg, err := range it {
		if err != nil {
			return stats, fmt.Errorf("iterating packages: %w", err)
		}
		if ctx.Err() != nil {
			return stats, context.Cause(ctx)
		}
		if err := enc.AddPackage(pkg.Package, agentVersion); err != nil {
			return stats, fmt.Errorf("encoding package %q: %w", pkg.Package.Name, err)
		}
		stats.Packages++
		stats.Functions += pkg.Stats().NumFunctions
		if err := maybeFlush(pkg.Final); err != nil {
			return stats, err
		}
	}
	return stats, nil
}
