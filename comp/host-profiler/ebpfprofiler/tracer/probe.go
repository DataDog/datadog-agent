// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tracer

import (
	"fmt"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"go.opentelemetry.io/ebpf-profiler/stringutil"
)

// ProbeType represents the type of eBPF probe.
type ProbeType int

const (
	// ProbeTypeKprobe represents a kernel probe.
	ProbeTypeKprobe ProbeType = iota
	// ProbeTypeKretprobe represents a kernel return probe.
	ProbeTypeKretprobe
	// ProbeTypeUprobe represents a user-space probe.
	ProbeTypeUprobe
	// ProbeTypeUretprobe represents a user-space return probe.
	ProbeTypeUretprobe
)

// String returns the string representation of the probe type.
func (p ProbeType) String() string {
	switch p {
	case ProbeTypeKprobe:
		return "kprobe"
	case ProbeTypeKretprobe:
		return "kretprobe"
	case ProbeTypeUprobe:
		return "uprobe"
	case ProbeTypeUretprobe:
		return "uretprobe"
	default:
		return "unknown"
	}
}

// ProbeSpec defines the specification for attaching an eBPF probe.
type ProbeSpec struct {
	// Type specifies the probe type (kprobe, kretprobe, uprobe, uretprobe).
	Type ProbeType
	// Target specifies the target executable path for user-space probes.
	Target string
	// Symbol specifies the function or symbol name to probe.
	Symbol string
	// ProgName specifies the eBPF program name to attach.
	ProgName string
}

const genericProgName = "kprobe__generic"

// ParseProbe parses a probe specification string and returns a ProbeSpec.
//
// The expected format is:
//   - For kernel probes: "kprobe:<symbol>" or "kretprobe:<symbol>"
//   - For user-space probes: "uprobe:<target>:<symbol>" or "uretprobe:<target>:<symbol>"
//
// The probe type is case-insensitive. Examples:
//   - "kprobe:do_sys_open"
//   - "kretprobe:tcp_connect"
//   - "uprobe:/usr/bin/bash:readline"
//   - "uretprobe:/lib/x86_64-linux-gnu/libc.so.6:malloc"
func ParseProbe(spec string) (*ProbeSpec, error) {
	var parts [3]string
	n := stringutil.SplitN(spec, ":", parts[:])
	if n < 2 {
		return nil, fmt.Errorf("invalid format: %s, expected: <probe_type>:<symbol>", spec)
	}

	probeTypeStr := strings.ToLower(parts[0])

	var probeType ProbeType
	switch probeTypeStr {
	case "kprobe":
		probeType = ProbeTypeKprobe
	case "kretprobe":
		probeType = ProbeTypeKretprobe
	case "uprobe":
		probeType = ProbeTypeUprobe
	case "uretprobe":
		probeType = ProbeTypeUretprobe
	default:
		return nil, fmt.Errorf("unknown probe type: %s", parts[0])
	}

	switch probeType {
	case ProbeTypeKprobe, ProbeTypeKretprobe:
		if n != 2 || parts[1] == "" {
			return nil, fmt.Errorf("invalid format: %s, expected: <probe_type>:<symbol>", spec)
		}
		return &ProbeSpec{
			Type:     probeType,
			Symbol:   parts[1],
			ProgName: genericProgName,
		}, nil

	case ProbeTypeUprobe, ProbeTypeUretprobe:
		if n != 3 || parts[2] == "" {
			return nil, fmt.Errorf("invalid format: %s, expected: <probe_type>:<target>:<symbol>", spec)
		}
		return &ProbeSpec{
			Type:     probeType,
			Target:   parts[1],
			Symbol:   parts[2],
			ProgName: genericProgName,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported probe type: %s", probeTypeStr)
	}
}

// AttachProbe attaches an eBPF program to the kernel or user-space based on the probe specification.
//
// It returns a link.Link that represents the attached probe, which can be used to
// detach the probe later by calling Close() on the link.
//
// For kernel probes (kprobe/kretprobe), it attaches to the specified kernel symbol.
// For user-space probes (uprobe/uretprobe), it attaches to the specified symbol in
// the target executable.
func AttachProbe(prog *ebpf.Program, spec *ProbeSpec) (link.Link, error) {
	switch spec.Type {
	case ProbeTypeKprobe:
		return link.Kprobe(spec.Symbol, prog, nil)
	case ProbeTypeKretprobe:
		return link.Kretprobe(spec.Symbol, prog, nil)
	case ProbeTypeUprobe:
		ex, err := link.OpenExecutable(spec.Target)
		if err != nil {
			return nil, err
		}
		return ex.Uprobe(spec.Symbol, prog, nil)
	case ProbeTypeUretprobe:
		ex, err := link.OpenExecutable(spec.Target)
		if err != nil {
			return nil, err
		}
		return ex.Uretprobe(spec.Symbol, prog, nil)
	}
	return nil, fmt.Errorf("unsupported probe type: %s", spec.Type)
}
