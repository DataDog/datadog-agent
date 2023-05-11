// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package protocols

import "github.com/DataDog/datadog-agent/pkg/network/config"

type ProtocolKind uint8

const (
	Http ProtocolKind = iota
)

type EbpfProgram interface {
	// ConfigureEbpfManager() error

	// PreStart()
	// PostStart()

	// PreStop()
	// PostStop()
}

// type Protocol[K comparable, V any] interface {
type Protocol interface {
	EbpfProgram
	// GetStats() map[K]V
}

type protocolFactory func(*config.Config) (Protocol, error)
type protocolFactoriesMap map[ProtocolKind]protocolFactory

var KnownProtocols = make(protocolFactoriesMap)

func RegisterProtocolFactory(protocolKind ProtocolKind, factory protocolFactory) {
	KnownProtocols[protocolKind] = factory
}
