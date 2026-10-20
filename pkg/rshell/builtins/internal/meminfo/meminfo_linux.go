// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package meminfo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// meminfoPath is the kernel pseudo-file read by readImpl. It is
// hardcoded — never derived from user input — so it is exempt from the
// AllowedPaths sandbox. See the package doc comment for the rationale.
const meminfoPath = "/proc/meminfo"

// maxMeminfoLine caps the per-line buffer size when scanning /proc/meminfo.
// Real lines are well under 80 bytes; this is a generous bound against a
// pathological /proc backend.
const maxMeminfoLine = 4096

// maxMeminfoLines caps the total number of lines scanned. Modern kernels
// report on the order of 50-60 lines; this bounds CPU time on a
// pathological input with many short lines.
const maxMeminfoLines = 1000

// readImpl opens and parses /proc/meminfo.
func readImpl(ctx context.Context) (Info, error) {
	f, err := os.Open(meminfoPath)
	if err != nil {
		return Info{}, fmt.Errorf("open %s: %w", meminfoPath, err)
	}
	defer f.Close() //nolint:errcheck

	info, err := parseMeminfo(ctx, f)
	if err != nil {
		return Info{}, fmt.Errorf("read %s: %w", meminfoPath, err)
	}
	return info, nil
}

// parseMeminfo reads meminfo-formatted lines from r and returns the
// resulting Info. Split out from readImpl so tests can exercise the
// parsing logic directly against a strings.Reader instead of the real
// /proc/meminfo.
func parseMeminfo(ctx context.Context, r io.Reader) (Info, error) {
	fields := make(map[string]uint64, 64)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024), maxMeminfoLine)

	lines := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return Info{}, err
		}
		lines++
		if lines > maxMeminfoLines {
			break
		}
		key, kib, ok := parseMeminfoLine(scanner.Text())
		if !ok {
			continue
		}
		fields[key] = kib
	}
	if err := scanner.Err(); err != nil {
		return Info{}, err
	}

	// /proc/meminfo reports every field in whole KiB; converting to bytes
	// is exact (no precision loss) and never overflows uint64 for any
	// realistic or even deliberately-inflated host memory size, but a
	// saturating multiply is used anyway as defense-in-depth against a
	// corrupted /proc backend reporting a huge value.
	toBytes := func(key string) uint64 {
		return mulSat(fields[key], 1024)
	}

	info := Info{
		MemTotal:     toBytes("MemTotal"),
		MemFree:      toBytes("MemFree"),
		Buffers:      toBytes("Buffers"),
		Cached:       toBytes("Cached"),
		SReclaimable: toBytes("SReclaimable"),
		Shared:       toBytes("Shmem"),
		SwapTotal:    toBytes("SwapTotal"),
		SwapFree:     toBytes("SwapFree"),
	}
	if _, ok := fields["MemAvailable"]; ok {
		info.MemAvailable = toBytes("MemAvailable")
	} else {
		// Pre-3.14 kernels do not report MemAvailable; approximate it the
		// way GNU free did before the kernel started computing it itself.
		info.MemAvailable = saturatingAdd(info.MemFree, saturatingAdd(info.Buffers, info.Cached))
	}
	return info, nil
}

// parseMeminfoLine parses one "Key:    Value kB" line from /proc/meminfo
// and returns the value in KiB. Lines without a " kB" suffix (e.g. the
// unitless HugePages_* counters) are skipped, since none of the fields
// free reports are unitless.
func parseMeminfoLine(line string) (key string, kib uint64, ok bool) {
	key, rest, found := strings.Cut(line, ":")
	if !found {
		return "", 0, false
	}
	numStr, hasUnit := strings.CutSuffix(strings.TrimSpace(rest), " kB")
	if !hasUnit {
		return "", 0, false
	}
	v, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return "", 0, false
	}
	return key, v, true
}

// mulSat returns a * b, clamped to uint64 max on overflow.
func mulSat(a, b uint64) uint64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > ^uint64(0)/b {
		return ^uint64(0)
	}
	return a * b
}
