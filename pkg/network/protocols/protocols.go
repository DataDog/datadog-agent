// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package protocols

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
)

// Programs maps used for tail calls
const (
	ProtocolDispatcherProgramsMap = "protocols_progs"
	TLSDispatcherProgramsMap      = "tls_process_progs"
	// TLSTerminationProgramsMap is the kprobe-typed mirror of the termination entries in
	// TLSDispatcherProgramsMap. USM's uprobes are loaded with expected_attach_type
	// BPF_TRACE_UPROBE_MULTI so they can share uprobe_multi links, and a PROG_ARRAY rejects
	// members whose expected_attach_type differs from its owner's, so kprobe__tcp_close needs
	// its own array to reach the termination programs.
	TLSTerminationProgramsMap                   = "tls_termination_progs"
	ProtocolDispatcherClassificationPrograms    = "dispatcher_classification_progs"
	TLSProtocolDispatcherClassificationPrograms = "tls_dispatcher_classification_progs"

	DefaultMapCleanerBatchSize = 100
)

// Protocol is the interface that represents a protocol supported by USM.
//
// Represents an eBPF program and provides methods used to manage its lifetime and initialisation.
type Protocol interface {
	// ConfigureOptions configures the provided Manager and Options structs with
	// additional options necessary for the program to work, such as options
	// depending on configuration values.
	ConfigureOptions(*manager.Options)

	// PreStart is called before the start of the provided eBPF manager.
	// Additional initialisation steps, such as starting an event consumer,
	// should be performed here.
	PreStart() error

	// PostStart is called after the start of the provided eBPF manager. Final
	// initialisation steps, such as setting up a map cleaner, should be
	// performed here.
	PostStart() error

	// Stop is called before the provided eBPF manager is stopped.  Cleanup
	// steps, such as stopping events consumers, should be performed here.
	// Note that since this method is a cleanup method, it *should not* fail and
	// tries to cleanup resources as best as it can.
	Stop()

	// DumpMaps dumps the content of the map represented by mapName &
	// currentMap, if it used by the eBPF program, to output.
	DumpMaps(w io.Writer, mapName string, currentMap *ebpf.Map)

	// Name returns the protocol name.
	Name() string

	// GetStats returns the latest monitoring stats from a protocol
	// implementation. The second return value is a callback for cleanup
	// purposes. Each protocol can use it to release resources, if necessary.
	GetStats() (*ProtocolStats, func())

	// IsBuildModeSupported return true is the given build mode is supported by this protocol.
	IsBuildModeSupported(buildmode.Type) bool
}

// ProtocolStats is a "tuple" struct that represents monitoring data from a
// Protocol implementation. It associates a ProtocolType and stats from this
// protocols' monitoring.
type ProtocolStats struct {
	Type  ProtocolType
	Stats interface{}
}

// ProtocolFactory is a function that creates a Protocol.
type ProtocolFactory func(*manager.Manager, *config.Config) (Protocol, error)

// ProtocolSpec represents a protocol specification.
type ProtocolSpec struct {
	Factory   ProtocolFactory
	Instance  Protocol
	Maps      []*manager.Map
	Probes    []*manager.Probe
	TailCalls []manager.TailCallRoute
}
