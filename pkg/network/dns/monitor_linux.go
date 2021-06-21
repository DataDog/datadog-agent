// +build linux_bpf

package dns

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	filterpkg "github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/ebpf/manager"
)

type dnsMonitor struct {
	*socketFilterSnooper
	p *ebpfProgram
}

// NewReverseDNS starts snooping on DNS traffic to allow IP -> domain reverse resolution
func NewReverseDNS(cfg *config.Config) (ReverseDNS, error) {
	p, err := newEBPFProgram(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating ebpf program: %w", err)
	}

	if err := p.Init(); err != nil {
		return nil, fmt.Errorf("error initializing ebpf programs: %w", err)
	}

	filter, _ := p.GetProbe(manager.ProbeIdentificationPair{Section: string(probes.SocketDnsFilter)})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	// Create the RAW_SOCKET inside the root network namespace
	var (
		packetSrc *filterpkg.AFPacketSource
		srcErr    error
	)
	err = util.WithRootNS(cfg.ProcRoot, func() error {
		packetSrc, srcErr = filterpkg.NewPacketSource(filter)
		return srcErr
	})
	if err != nil {
		return nil, err
	}

	snoop, err := newSocketFilterSnooper(cfg, packetSrc)
	if err != nil {
		return nil, err
	}
	return &dnsMonitor{
		snoop,
		p,
	}, nil
}

// Close releases associated resources
func (m *dnsMonitor) Close() {
	m.socketFilterSnooper.Close()
	_ = m.p.Stop(manager.CleanAll)
}
