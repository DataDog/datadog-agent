// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test

package tests

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
)

// cleanProtocolMapByProtocol cleans up the protocol map for a given protocol
func cleanProtocolMapByProtocol(t *testing.T, tr *tracer.Tracer, protocol protocols.ProtocolType) {
	selector := func(_ netebpf.ConnTuple, wrapper netebpf.ProtocolStackWrapper) bool {
		return wrapper.Stack.Application == uint8(protocol) || wrapper.Stack.Api == uint8(protocol) || wrapper.Stack.Encryption == uint8(protocol)
	}
	cleanProtocolMapBySelector(t, tr, selector)
	cleanConnMapBySelector(t, tr, selector)
}

// cleanProtocolMapBySelector cleans up the protocol map for a given selector
func cleanProtocolMapBySelector(t *testing.T, tr *tracer.Tracer, selector func(netebpf.ConnTuple, netebpf.ProtocolStackWrapper) bool) {
	protocolMap, err := tr.GetMap(probes.ConnectionProtocolMap)
	require.NoError(t, err)

	keysToDelete := make([]netebpf.ConnTuple, 0)

	var key netebpf.ConnTuple
	var value netebpf.ProtocolStackWrapper
	iter := protocolMap.Iterate()
	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
		if selector(key, value) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		// best effort deletion
		_ = protocolMap.Delete(unsafe.Pointer(&key))
	}
}

// cleanConnMapBySelector cleans up the protocol map for a given selector
func cleanConnMapBySelector(t *testing.T, tr *tracer.Tracer, selector func(netebpf.ConnTuple, netebpf.ProtocolStackWrapper) bool) {
	connMap, err := tr.GetMap(probes.ConnMap)
	require.NoError(t, err)

	keysToDelete := make([]netebpf.ConnTuple, 0)

	var key netebpf.ConnTuple
	var value netebpf.ConnStats
	iter := connMap.Iterate()
	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
		if selector(key, netebpf.ProtocolStackWrapper{Stack: value.Protocol_stack}) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		// best effort deletion
		_ = connMap.Delete(unsafe.Pointer(&key))
	}
}
