// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package usm

import (
	"io"
	"testing"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/usm/buildmode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Helper type to wrap & mock Protocols in tests. We keep an instance of the
// inner protocol to be able to call ConfigureOptions.
type protocolMock struct {
	inner protocols.Protocol
	spec  protocolMockSpec
}

// Helper type to specify in tests which methods to replace in the mock.
type protocolMockSpec struct {
	// These functions can be set to change the behavior of those methods. If
	// not set, the methods from the base protocol will be called.
	preStartFn  func() error
	postStartFn func() error
	stopFn      func()
}

// Name return the program's name.
func (p *protocolMock) Name() string {
	return "mock"
}

// ConfigureOptions changes map attributes to the given options.
func (p *protocolMock) ConfigureOptions(opts *manager.Options) {
	p.inner.ConfigureOptions(opts)
}

func (p *protocolMock) PreStart() (err error) {
	if p.spec.preStartFn != nil {
		return p.spec.preStartFn()
	}
	return p.inner.PreStart()
}

func (p *protocolMock) PostStart() error {
	if p.spec.postStartFn != nil {
		return p.spec.postStartFn()
	}
	return p.inner.PostStart()
}

func (p *protocolMock) Stop() {
	if p.spec.stopFn != nil {
		p.Stop()
	} else {
		p.inner.Stop()
	}
}

func (p *protocolMock) DumpMaps(io.Writer, string, *ebpf.Map)        {}
func (p *protocolMock) GetStats() (*protocols.ProtocolStats, func()) { return nil, nil }

// IsBuildModeSupported returns always true, as the mock is supported by all modes.
func (*protocolMock) IsBuildModeSupported(buildmode.Type) bool { return true }

// patchProtocolMock updates the map of known protocols to replace the mock
// factory in place of the HTTP protocol factory
func patchProtocolMock(t *testing.T, spec protocolMockSpec) {
	t.Helper()

	p := knownProtocols[0]
	innerFactory := p.Factory

	// Restore the old protocol factory at end of test
	t.Cleanup(func() {
		p.Factory = innerFactory
		knownProtocols[0] = p
	})

	p.Factory = func(m *manager.Manager, c *config.Config) (protocols.Protocol, error) {
		inner, err := innerFactory(m, c)
		if err != nil {
			return nil, err
		}

		return &protocolMock{
			inner,
			spec,
		}, nil
	}

	knownProtocols[0] = p
}

// SetConnectionProtocol sets the connection protocol for the given connection tuple.
func (m *Monitor) SetConnectionProtocol(t *testing.T, p netebpf.ProtocolStackWrapper, tup netebpf.ConnTuple) {
	t.Helper()

	connProtocolMap, _, err := m.ebpfProgram.GetMap(probes.ConnectionProtocolMap)
	require.NoError(t, err)
	require.NoError(t, connProtocolMap.Update(unsafe.Pointer(&tup), unsafe.Pointer(&p), ebpf.UpdateAny))
}

// skipIfKernelNotSupported skips the test if the current kernel version is below the minimum required version.
func skipIfKernelNotSupported(t *testing.T, minimumKernelVersion kernel.Version, protocolName string) {
	t.Helper()
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < minimumKernelVersion {
		t.Skipf("%s monitoring can not run on kernel before %v", protocolName, minimumKernelVersion)
	}
}
