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
	preStartFn  func(*manager.Manager, protocols.BuildMode) error
	postStartFn func(*manager.Manager) error
	stopFn      func(*manager.Manager)
}

func (p *protocolMock) Name() string {
	return "mock"
}

func (p *protocolMock) ConfigureOptions(m *manager.Manager, opts *manager.Options) {
	p.inner.ConfigureOptions(m, opts)
}

func (p *protocolMock) PreStart(mgr *manager.Manager, mode protocols.BuildMode) (err error) {
	if p.spec.preStartFn != nil {
		return p.spec.preStartFn(mgr, mode)
	} else {
		return p.inner.PreStart(mgr, mode)
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

func (p *protocolMock) DumpMaps(*strings.Builder, string, *ebpf.Map) {}
func (p *protocolMock) GetStats() *protocols.ProtocolStats           { return nil }

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

	knownProtocols[0] = p
}
