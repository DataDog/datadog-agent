// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/socket-contention-kern.c pkg/ebpf/bytecode/build/runtime/socket-contention.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/socket-contention.c pkg/ebpf/bytecode/runtime/socket-contention.go runtime

package socketcontention

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/socketcontention/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statsMapName          = "socket_contention_stats"
	socketContentionGroup = "socket_contention"
)

var minimumKernelVersion = kernel.VersionCode(5, 5, 0)

func contentionTracepointsSupported() bool {
	traceFSRoot, err := tracefs.Root()
	if err != nil {
		return false
	}

	if _, err := os.Stat(filepath.Join(traceFSRoot, "events/lock/contention_begin/id")); errors.Is(err, os.ErrNotExist) {
		return false
	}

	if _, err := os.Stat(filepath.Join(traceFSRoot, "events/lock/contention_end/id")); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// Probe is the eBPF side of the socket contention check.
type Probe struct {
	objects *bpfObjects
	links   []link.Link
}

// NewProbe creates a [Probe].
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("detect kernel version: %w", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}
	if !contentionTracepointsSupported() {
		return nil, fmt.Errorf("lock contention tracepoints are not available on this kernel")
	}

	var probe *Probe
	err = ddebpf.LoadCOREAsset("socket-contention.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load CO-RE socket contention probe: %w", err)
	}

	return probe, nil
}

type bpfPrograms struct {
	TpContentionBegin *ebpf.Program `ebpf:"tp_contention_begin"`
	TpContentionEnd   *ebpf.Program `ebpf:"tp_contention_end"`
}

type bpfMaps struct {
	Stats *ebpf.Map `ebpf:"socket_contention_stats"`
}

type bpfObjects struct {
	bpfPrograms
	bpfMaps
}

func startProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*Probe, error) {
	p := &Probe{
		objects: new(bpfObjects),
	}

	collectionSpec, err := ebpf.LoadCollectionSpecFromReader(buf)
	if err != nil {
		return nil, fmt.Errorf("load collection spec: %w", err)
	}

	if spec, ok := collectionSpec.Maps["tstamp"]; ok {
		spec.MaxEntries = 1024
	}

	opts := ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
		},
	}

	if err := collectionSpec.LoadAndAssign(p.objects, &opts); err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("verifier error loading socket contention collection: %s\n%+v", err, ve)
		}
		return nil, fmt.Errorf("load socket contention objects: %w", err)
	}

	tpContentionBegin, err := link.AttachTracing(link.TracingOptions{
		Program: p.objects.TpContentionBegin,
	})
	if err != nil {
		return nil, fmt.Errorf("attach contention begin tracepoint: %w", err)
	}
	p.links = append(p.links, tpContentionBegin)

	tpContentionEnd, err := link.AttachTracing(link.TracingOptions{
		Program: p.objects.TpContentionEnd,
	})
	if err != nil {
		tpContentionBegin.Close()
		return nil, fmt.Errorf("attach contention end tracepoint: %w", err)
	}
	p.links = append(p.links, tpContentionEnd)

	ddebpf.AddNameMappingsForMap(p.objects.Stats, statsMapName, socketContentionGroup)
	ddebpf.AddNameMappingsForProgram(p.objects.TpContentionBegin, "tracepoint__contention_begin", socketContentionGroup)
	ddebpf.AddNameMappingsForProgram(p.objects.TpContentionEnd, "tracepoint__contention_end", socketContentionGroup)

	return p, nil
}

// Close releases all associated resources.
func (p *Probe) Close() {
	if p == nil || p.objects == nil {
		return
	}

	for _, ebpfLink := range p.links {
		if err := ebpfLink.Close(); err != nil {
			log.Warnf("error closing socket contention link: %s", err)
		}
	}

	ddebpf.RemoveNameMappingsCollection(&ebpf.Collection{
		Maps: map[string]*ebpf.Map{
			statsMapName: p.objects.Stats,
		},
		Programs: map[string]*ebpf.Program{
			"tracepoint__contention_begin": p.objects.TpContentionBegin,
			"tracepoint__contention_end":   p.objects.TpContentionEnd,
		},
	})

	if p.objects.Stats != nil {
		p.objects.Stats.Close()
	}
	if p.objects.TpContentionBegin != nil {
		p.objects.TpContentionBegin.Close()
	}
	if p.objects.TpContentionEnd != nil {
		p.objects.TpContentionEnd.Close()
	}
}

// GetAndFlush gets the current stats and clears the map.
func (p *Probe) GetAndFlush() model.SocketContentionStats {
	key := uint32(0)
	var raw ebpfSocketContentionStats
	if err := p.objects.Stats.Lookup(&key, &raw); err != nil {
		return model.SocketContentionStats{}
	}

	if err := p.objects.Stats.Delete(&key); err != nil {
		log.Warnf("failed to delete socket contention stat: %s", err)
	}

	return model.SocketContentionStats{
		TotalTimeNS: raw.Total_time_ns,
		MinTimeNS:   raw.Min_time_ns,
		MaxTimeNS:   raw.Max_time_ns,
		Count:       raw.Count,
		Flags:       raw.Flags,
	}
}
