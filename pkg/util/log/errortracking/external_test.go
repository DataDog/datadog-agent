// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package errortracking

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func externalRecord(ext ExternalStack, attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(time.Now(), slog.LevelError, "msg", 0)
	r.AddAttrs(attrs...)
	r.AddAttrs(slog.Any("integration", ext))
	return r
}

func TestHandler_ExternalStack(t *testing.T) {
	ext := ExternalStack{
		Frames:    "pkg.mod.func\n\tpkg/mod.py:12 +0x0",
		ErrorKind: "ValueError",
		Source:    "mod",
	}

	t.Run("submits the pre-formatted stack instead of a Go stack", func(t *testing.T) {
		h, rec := newRecordingHandler(t)
		require.NoError(t, h.Handle(context.Background(), externalRecord(ext, slog.Int("n", 1))))

		got := rec.snapshot()
		require.Len(t, got, 1)
		assert.Equal(t, ext.Frames, got[0].Stack)
		assert.Equal(t, "ValueError", got[0].ErrorKind)
		assert.Equal(t, "mod", got[0].Integration)
		assert.Zero(t, got[0].PCsLen, "no Go stack must be captured")
		assert.Equal(t, uint32(1), got[0].Count)
	})

	t.Run("same stack is deduplicated, a different stack is not", func(t *testing.T) {
		h, rec := newRecordingHandler(t)
		other := ext
		other.Frames = "pkg.mod.other\n\tpkg/mod.py:99 +0x0"
		otherKind := ext
		otherKind.ErrorKind = "KeyError"

		for _, e := range []ExternalStack{ext, ext, other, otherKind} {
			require.NoError(t, h.Handle(context.Background(), externalRecord(e)))
		}
		assert.Len(t, rec.snapshot(), 3, "second identical stack must be suppressed")
	})

	t.Run("dropped without a bouncer", func(t *testing.T) {
		h, rec := newRecordingHandler(t)
		h = h.WithBouncerLoader(func() *Bouncer { return nil })
		require.NoError(t, h.Handle(context.Background(), externalRecord(ext)))
		assert.Empty(t, rec.snapshot())
	})

	t.Run("records without the attribute keep the Go stack path", func(t *testing.T) {
		h, rec := newRecordingHandler(t)
		r := slog.NewRecord(time.Now(), slog.LevelError, "msg", 0xC0FFEE)
		r.AddAttrs(slog.String("integration", "mod"))
		require.NoError(t, h.Handle(context.Background(), r))

		got := rec.snapshot()
		require.Len(t, got, 1)
		assert.Empty(t, got[0].Stack)
		assert.Equal(t, uintptr(0xC0FFEE), got[0].PC)
	})
}

func TestExternalStack_StringHidesFrames(t *testing.T) {
	ext := ExternalStack{Frames: "a\n\tb.py:1 +0x0", ErrorKind: "ValueError", Source: "redisdb"}
	assert.Equal(t, "redisdb", ext.String())
}
