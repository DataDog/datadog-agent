// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const maxTelemetrySpans = 10_000

// Finished spans wait in the dependency's process-global queue until a sender
// flushes them, so tag values must stay small no matter how large the script.
const (
	maxRunCommandTagBytes   = 4 << 10
	maxCommandFlagsTagBytes = 1 << 10
	telemetryTruncatedMark  = "...[truncated]"
)

var (
	// The dependency tracer is process-global, so the cap must be too. This
	// counter is never reset because rshell cannot observe when its queue flushes.
	telemetrySpansStarted atomic.Uint64
	droppedTelemetrySpan  rshellTelemetrySpan = noOpTelemetrySpan{}
)

type rshellTelemetrySpan interface {
	Finish(error)
	SetResourceName(string)
	SetTag(string, any)
}

type noOpTelemetrySpan struct{}

func (noOpTelemetrySpan) Finish(error)           {}
func (noOpTelemetrySpan) SetResourceName(string) {}
func (noOpTelemetrySpan) SetTag(string, any)     {}

func startTelemetrySpan(ctx context.Context, operationName string) (rshellTelemetrySpan, context.Context) {
	for {
		started := telemetrySpansStarted.Load()
		if started >= maxTelemetrySpans {
			return droppedTelemetrySpan, ctx
		}
		if telemetrySpansStarted.CompareAndSwap(started, started+1) {
			return telemetry.StartSpanFromContext(ctx, operationName)
		}
	}
}

// truncateTelemetryTag returns text cut to at most limit bytes on a UTF-8
// boundary, followed by telemetryTruncatedMark when anything was dropped. The
// result never shares memory with text, so a queued span cannot keep a large
// script alive.
func truncateTelemetryTag(text string, limit int) string {
	var b strings.Builder
	if len(text) <= limit {
		b.WriteString(text)
		return b.String()
	}
	cut := limit
	// Back off over UTF-8 continuation bytes (10xxxxxx) to a rune start.
	for cut > 0 && text[cut]&0xC0 == 0x80 {
		cut--
	}
	b.Grow(cut + len(telemetryTruncatedMark))
	b.WriteString(text[:cut])
	b.WriteString(telemetryTruncatedMark)
	return b.String()
}

// commandFlagsTag joins flags into the comma-padded rshell.command.flags value.
// Flags are never cut mid-name, so an exact "*,-n,*" query stays reliable; the
// first flag that would push the value past maxCommandFlagsTagBytes ends the
// list. It returns "" when no flag fits.
func commandFlagsTag(flags []string) string {
	var b strings.Builder
	for _, flag := range flags {
		// Each flag adds its trailing comma; only the first also adds the
		// leading one.
		need := len(flag) + 1
		if b.Len() == 0 {
			need++
		}
		if b.Len()+need > maxCommandFlagsTagBytes {
			break
		}
		if b.Len() == 0 {
			b.WriteByte(',')
		}
		b.WriteString(flag)
		b.WriteByte(',')
	}
	return b.String()
}
