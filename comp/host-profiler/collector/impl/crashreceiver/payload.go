// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package crashreceiver

import (
	"crypto/sha256"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pprofile"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/reporter/samples"
)

func buildCrashProfile(trace *libpf.Trace, meta *samples.TraceEventMeta) pprofile.Profiles {
	profiles := pprofile.NewProfiles()
	rp := profiles.ResourceProfiles().AppendEmpty()
	attrs := rp.Resource().Attributes()

	if s := meta.ContainerID.String(); s != "" {
		attrs.PutStr("container.id", s)
	}
	if meta.APMServiceName != "" {
		attrs.PutStr("service.name", meta.APMServiceName)
	}
	for k, v := range meta.EnvVars {
		attrs.PutStr(k.String(), v.String())
	}

	attrs.PutInt("crash.signal", meta.Value)
	attrs.PutInt("crash.pid", int64(meta.PID))
	attrs.PutStr("crash.comm", meta.Comm.String())
	attrs.PutInt("crash.timestamp_ms", int64(meta.Timestamp)/1e6)
	attrs.PutStr("crash.service", resolveService(meta))
	attrs.PutStr("crash.fingerprint", fingerprint(trace.Frames))
	attrs.PutStr("crash.language", detectLanguage(trace.Frames))

	frames := attrs.PutEmptySlice("crash.frames")
	for _, fh := range trace.Frames {
		f := fh.Value()
		m := frames.AppendEmpty().SetEmptyMap()
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

	rp.ScopeProfiles().AppendEmpty().Profiles().AppendEmpty()
	return profiles
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
	for _, h := range frames {
		ft := h.Value().Type
		if ft != libpf.KernelFrame {
			counts[ft]++
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
