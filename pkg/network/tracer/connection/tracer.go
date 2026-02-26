// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connection

import (
	"io"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// TracerType is the type of the underlying tracer
type TracerType int

const (
	// TracerTypeKProbePrebuilt is the TracerType for prebuilt kprobe tracer
	TracerTypeKProbePrebuilt TracerType = iota
	// TracerTypeKProbeRuntimeCompiled is the TracerType for the runtime compiled kprobe tracer
	TracerTypeKProbeRuntimeCompiled
	// TracerTypeKProbeCORE is the TracerType for the CORE kprobe tracer
	TracerTypeKProbeCORE
	// TracerTypeFentry is the TracerType for the fentry tracer
	TracerTypeFentry
	// TracerTypeEbpfless is the TracerType for the EBPF-less tracer
	TracerTypeEbpfless
	// TracerTypeDarwin is the TracerType for the Darwin tracer (uses ebpfless implementation)
	TracerTypeDarwin
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 512 //nolint:unused // used by Linux eBPF tracer
)

// Tracer is the common interface implemented by all connection tracers.
type Tracer interface {
	// Start begins collecting network connection data.
	Start(func(*network.ConnectionStats)) error
	// Stop halts all network data collection.
	Stop()
	// GetConnections returns the list of currently active connections, using the buffer provided.
	// The optional filter function is used to prevent unwanted connections from being returned and consuming resources.
	GetConnections(buffer *network.ConnectionBuffer, filter func(*network.ConnectionStats) bool) error
	// FlushPending forces any closed connections waiting for batching to be processed immediately.
	FlushPending()
	// Remove deletes the connection from tracking state.
	// It does not prevent the connection from re-appearing later, if additional traffic occurs.
	Remove(conn *network.ConnectionStats) error
	// GetMap returns the underlying named map. This is useful if any maps are shared with other eBPF components.
	// An individual tracer implementation may choose which maps to expose via this function.
	GetMap(string) (*ebpf.Map, error)
	// DumpMaps (for debugging purpose) returns all maps content by default or selected maps from maps parameter.
	DumpMaps(w io.Writer, maps ...string) error
	// Type returns the type of the underlying ebpf tracer that is currently loaded
	Type() TracerType

	Pause() error
	Resume() error

	// Describe returns all descriptions of the collector
	Describe(descs chan<- *prometheus.Desc)
	// Collect returns the current state of all metrics of the collector
	Collect(metrics chan<- prometheus.Metric)
}
