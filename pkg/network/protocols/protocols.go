// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package protocols

import (
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

type ProtocolKind uint8

const (
	Http ProtocolKind = iota
)

const ProtocolDispatcherProgramsMap = "protocols_progs"

type EbpfProgram interface {
	ConfigureOptions(*manager.Manager, *manager.Options)

	PreStart(*manager.Manager) error
	PostStart(*manager.Manager)

	PreStop(*manager.Manager)
	PostStop(*manager.Manager)

	DumpMaps(output *strings.Builder, mapName string, currentMap *ebpf.Map)
}

type ProtocolStats struct {
	Kind  ProtocolKind
	Stats interface{}
}

type Protocol interface {
	EbpfProgram
	GetStats() *ProtocolStats
}

type protocolFactory func(*config.Config) (Protocol, error)
type ProtocolSpec struct {
	Factory   protocolFactory
	Maps      []*manager.Map
	TailCalls []manager.TailCallRoute
}
type protocolSpecsMap map[ProtocolKind]ProtocolSpec

var KnownProtocols = make(protocolSpecsMap)

func RegisterProtocol(protocolKind ProtocolKind, spec ProtocolSpec) {
	KnownProtocols[protocolKind] = spec
}

func AddBoolConst(options *manager.Options, flag bool, name string) {
	val := uint64(1)
	if !flag {
		val = uint64(0)
	}

	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: val,
		},
	)
}
