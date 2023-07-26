// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"strings"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
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
	preStartFn  func(*manager.Manager) error
	postStartFn func(*manager.Manager) error
	stopFn      func(*manager.Manager)
}

func (p *protocolMock) ConfigureOptions(m *manager.Manager, opts *manager.Options) {
	p.inner.ConfigureOptions(m, opts)
}

func (p *protocolMock) PreStart(mgr *manager.Manager) (err error) {
	if p.spec.preStartFn != nil {
		return p.spec.preStartFn(mgr)
	} else {
		return p.inner.PreStart(mgr)
	}
}

func (p *protocolMock) PostStart(mgr *manager.Manager) error {
	if p.spec.postStartFn != nil {
		return p.spec.postStartFn(mgr)
	} else {
		return p.inner.PostStart(mgr)
	}
}

func (p *protocolMock) Stop(mgr *manager.Manager) {
	if p.spec.stopFn != nil {
		p.Stop(mgr)
	} else {
		p.inner.Stop(mgr)
	}
}

func (p *protocolMock) DumpMaps(output *strings.Builder, mapName string, currentMap *ebpf.Map) {}
func (p *protocolMock) GetStats() *protocols.ProtocolStats                                     { return nil }

// patchProtocolMock updates the map of known protocols to replace the mock
// factory in place of the HTTP protocol factory
func patchProtocolMock(t *testing.T, protocolType protocols.ProtocolType, spec protocolMockSpec) {
	t.Helper()

	p, present := knownProtocols[protocolType.String()]
	require.True(t, present, "trying to patch non-existing protocol")

	innerFactory := p.Factory

	// Restore the old protocol factory at end of test
	t.Cleanup(func() {
		p.Factory = innerFactory
		knownProtocols[protocolType.String()] = p
	})

	p.Factory = func(c *config.Config) (protocols.Protocol, error) {
		inner, err := innerFactory(c)
		if err != nil {
			return nil, err
		}

		return &protocolMock{
			inner,
			spec,
		}, nil
	}

	knownProtocols[protocolType.String()] = p
}
