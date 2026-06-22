// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package crashreceiver

import (
	"crypto/sha256"
	"fmt"
	"strings"


	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/reporter/samples"
)

// buildBundledProfile builds a pprofile.Profiles from all threads of a single
// crashing process. The primary thread (TID==PID if present, otherwise first)
// is encoded in crash.frames; remaining threads go in crash.threads.
func buildBundledProfile(events []*threadEvent) pprofile.Profiles {
	profiles := pprofile.NewProfiles()
	if len(events) == 0 {
		return profiles
	}

	// Identify the primary thread: prefer the one with the most language-specific
	// (non-kernel, non-native) frames — that's the goroutine/thread doing user
	// work. For Go processes TID==PID is typically the scheduler, not the
	// allocating goroutine. Fall back to TID==PID, then first event.
	primaryIdx := bestThreadIdx(events)
	primary := events[primaryIdx]

	rp := profiles.ResourceProfiles().AppendEmpty()
	attrs := rp.Resource().Attributes()

	if s := primary.meta.ContainerID.String(); s != "" {
		attrs.PutStr("container.id", s)
	}
	if primary.meta.APMServiceName != "" {
		attrs.PutStr("service.name", primary.meta.APMServiceName)
	}
	for k, v := range primary.meta.EnvVars {
		attrs.PutStr(k.String(), v.String())
	}

	attrs.PutInt("crash.signal", primary.meta.Value)
	attrs.PutInt("crash.pid", int64(primary.meta.PID))
	attrs.PutStr("crash.comm", primary.meta.Comm.String())
	attrs.PutInt("crash.timestamp_ms", int64(primary.meta.Timestamp)/1e6)
	attrs.PutStr("crash.service", resolveService(primary.meta))
	attrs.PutStr("crash.fingerprint", fingerprint(primary.trace.Frames))
	attrs.PutStr("crash.language", detectLanguage(primary.trace.Frames))

	encodeFrames(attrs.PutEmptySlice("crash.frames"), primary.trace.Frames)

	// Non-primary threads go in crash.threads.
	if len(events) > 1 {
		threads := attrs.PutEmptySlice("crash.threads")
		for i, ev := range events {
			if i == primaryIdx {
				continue
			}
			t := threads.AppendEmpty().SetEmptyMap()
			t.PutStr("name", ev.meta.Comm.String())
			t.PutInt("tid", int64(ev.meta.TID))
			encodeFrames(t.PutEmptySlice("frames"), ev.trace.Frames)
		}
	}

	rp.ScopeProfiles().AppendEmpty().Profiles().AppendEmpty()
	return profiles
}

// encodeFrames encodes a libpf.Frames slice into a pcommon.Slice of Maps,
// matching oomtrace's toStackFrame field layout.
func encodeFrames(sl pcommon.Slice, frames libpf.Frames) {
	for _, fh := range frames {
		f := fh.Value()
		m := sl.AppendEmpty().SetEmptyMap()
		m.PutStr("function", f.FunctionName.String())
		m.PutStr("file", f.SourceFile.String())
		if f.SourceLine > 0 {
			m.PutInt("line", int64(f.SourceLine))
		}
		if f.SourceColumn > 0 {
			m.PutInt("column", int64(f.SourceColumn))
		}
		if f.Mapping.Valid() {
			mv := f.Mapping.Value()
			file := mv.File.Value()
			m.PutStr("path", file.FileName.String())
			m.PutStr("file_type", "ELF")
			if bid := file.GnuBuildID; bid != "" {
				m.PutStr("build_id", bid)
				m.PutStr("build_id_type", "GNU")
			} else if bid := file.GoBuildID; bid != "" {
				m.PutStr("build_id", bid)
				m.PutStr("build_id_type", "GO")
			}
			if f.AddressOrLineno > 0 {
				m.PutStr("relative_address", fmt.Sprintf("0x%x", f.AddressOrLineno))
			}
		}
	}
}

// isGoFrame reports whether a frame originates from Go code.
// Go compiles to native code, so frames can appear in three ways:
//  1. GoFrame type — from the profiler's Go tracer (CGO shared libraries)
//  2. NativeFrame with Go build ID — pure Go binary, mapping indexed
//  3. NativeFrame with runtime.* function name — pure Go binary, function
//     name resolved from DWARF (most reliable fallback when build ID absent)
func isGoFrame(f libpf.Frame) bool {
	if f.Type == libpf.GoFrame {
		return true
	}
	if f.Type == libpf.NativeFrame {
		if f.Mapping.Valid() && f.Mapping.Value().File.Value().GoBuildID != "" {
			return true
		}
		fn := f.FunctionName.String()
		return fn != "" && strings.HasPrefix(fn, "runtime.")
	}
	return false
}

// bestThreadIdx returns the index of the most interesting thread.
// Interpreter frames (Python, Ruby, JVM, etc.) always score — they are by
// definition user code. NativeFrame gets the runtime.* filter because that
// is where Go compiles to: user Go code (main.*) scores, GC/scheduler
// goroutines (runtime.* only) score zero.
func bestThreadIdx(events []*threadEvent) int {
	bestIdx := 0
	bestScore := -1

	for i, ev := range events {
		score := 0
		hasPageFault := false
		for _, fh := range ev.trace.Frames {
			f := fh.Value()
			ft := f.Type
			fn := f.FunctionName.String()
			if ft == libpf.KernelFrame {
				if fn == "exc_page_fault" {
					hasPageFault = true
				}
				continue
			}
			if ft == libpf.NativeFrame {
				if fn != "" && !strings.HasPrefix(fn, "runtime.") {
					score++
				}
			} else {
				// PythonFrame, RubyFrame, GoFrame, HotSpotFrame, V8Frame, etc.
				score++
			}
		}
		// Encode page-fault presence in the score so it acts as a tiebreaker
		// when user-space names are unresolved (all primary scores zero).
		// exc_page_fault means this thread was actively accessing memory at
		// crash time — the allocating goroutine for OOM, the faulting thread
		// for SIGSEGV.
		if hasPageFault {
			score += 1000
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return bestIdx
}

func resolveService(meta *samples.TraceEventMeta) string {
	if meta.APMServiceName != "" {
		return meta.APMServiceName
	}
	if meta.EnvVars != nil {
		if v, ok := meta.EnvVars[libpf.Intern("DD_SERVICE")]; ok {
			if s := v.String(); s != "" {
				return s
			}
		}
	}
	return "unknown"
}

func fingerprint(frames libpf.Frames) string {
	h := sha256.New()
	n := 0
	for _, fh := range frames {
		if n >= 5 {
			break
		}
		f := fh.Value()
		if f.Type == libpf.KernelFrame {
			continue
		}
		if name := f.FunctionName.String(); name != "" {
			fmt.Fprintf(h, "fn:%s\n", name)
			n++
		} else if f.Mapping.Valid() {
			file := f.Mapping.Value().File.Value()
			bid := file.GnuBuildID
			if bid == "" {
				bid = file.GoBuildID
			}
			if bid != "" && f.AddressOrLineno > 0 {
				fmt.Fprintf(h, "elf:%s:0x%x\n", bid, f.AddressOrLineno)
				n++
			}
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func detectLanguage(frames libpf.Frames) string {
	counts := map[libpf.FrameType]int{}
	hasGoBinary := false
	for _, h := range frames {
		f := h.Value()
		ft := f.Type
		if ft != libpf.KernelFrame {
			counts[ft]++
		}
		if !hasGoBinary && isGoFrame(f) {
			hasGoBinary = true
		}
	}
	best := libpf.NativeFrame
	bestCount := 0
	for ft, c := range counts {
		if ft == libpf.NativeFrame {
			continue
		}
		if c > bestCount {
			best = ft
			bestCount = c
		}
	}
	// Go compiles to native code so its frames appear as NativeFrame.
	// If no interpreted language dominates, check for Go via build ID.
	if bestCount == 0 && hasGoBinary {
		return "go"
	}
	return frameTypeLanguage(best)
}

func frameTypeLanguage(ft libpf.FrameType) string {
	switch ft {
	case libpf.GoFrame:
		return "go"
	case libpf.PythonFrame:
		return "python"
	case libpf.HotSpotFrame:
		return "jvm"
	case libpf.RubyFrame:
		return "ruby"
	case libpf.DotnetFrame:
		return "dotnet"
	case libpf.V8Frame:
		return "nodejs"
	case libpf.PHPFrame, libpf.PHPJITFrame:
		return "php"
	case libpf.PerlFrame:
		return "perl"
	default:
		return "native"
	}
}
