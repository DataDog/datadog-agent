// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"fmt"
	"slices"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	probeUID = "dns"
	// dnsPortsMax is the maximum number of distinct DNS ports the filter
	// can monitor. Matches DNS_PORTS_MAX in pkg/network/ebpf/c/prebuilt/dns.c.
	// 8 covers every realistic configuration (53 + mDNS/LLMNR + the
	// 1053/8053/9053/10053 unprivileged CoreDNS family + a spare slot).
	// The limit applies AFTER deduplication, so repeated entries do not
	// consume slots. Configurations with more than dnsPortsMax distinct
	// entries are TRUNCATED to the first dnsPortsMax ports sorted ascending
	// rather than failing startup — the prior BPF_MAP_TYPE_HASH allowed
	// up to 32 entries, but no published documentation specified a maximum,
	// so the in-the-wild distribution is unknown. A loud WARN log + the
	// dnsMonitorTelemetry.portsTruncated counter make the truncation
	// observable; if telemetry surfaces any customer with > 8 ports we can
	// raise the cap or restore an error.
	dnsPortsMax       = 8
	dnsMonitorTelName = "dns_monitor"
)

// dnsMonitorTelemetry holds counters surfaced via agent telemetry.
var dnsMonitorTelemetry = struct {
	portsTruncated telemetry.Counter
}{
	telemetryimpl.GetCompatComponent().NewCounter(
		dnsMonitorTelName, "ports_truncated", nil,
		"Times the configured DNS port list exceeded the BPF slot capacity and was truncated at startup",
	),
}

type ebpfProgram struct {
	*manager.Manager
	cfg      *config.Config
	bytecode bytecode.AssetReader
}

func newEBPFProgram(c *config.Config) (*ebpfProgram, error) {
	bc, err := netebpf.ReadDNSModule(c.BPFDir, c.BPFDebug)
	if err != nil {
		return nil, err
	}

	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probes.SocketDNSFilter,
					UID:          probeUID,
				},
			},
		},
	}

	return &ebpfProgram{
		Manager:  mgr,
		bytecode: bc,
		cfg:      c,
	}, nil
}

func (e *ebpfProgram) Init() error {
	defer e.bytecode.Close()

	ports := slices.Clone(e.cfg.DNSMonitoringPortList)
	for _, p := range ports {
		if p <= 0 || p > 65535 {
			return fmt.Errorf("network_config.dns_monitoring_ports contains invalid port %d (must be 1-65535)", p)
		}
	}
	// Sort + deduplicate before applying the slot cap. The prior
	// BPF_MAP_TYPE_HASH naturally de-duplicated keys via repeated Put calls,
	// so a config like [53, 53, 5353] consumed only two slots — preserve
	// that behavior here so duplicate config entries don't artificially
	// inflate against dnsPortsMax.
	slices.Sort(ports)
	ports = slices.Compact(ports)
	if len(ports) > dnsPortsMax {
		// Truncate rather than fail. Sorted-ascending order means the
		// lower-numbered ports (typically 53 + mDNS/LLMNR + unprivileged
		// CoreDNS family) win; higher arbitrary user ports are the ones
		// dropped. Loud WARN + telemetry counter make this observable.
		dropped := slices.Clone(ports[dnsPortsMax:])
		ports = ports[:dnsPortsMax]
		log.Warnf(
			"network_config.dns_monitoring_ports has %d distinct entries, exceeding the maximum of %d. "+
				"Monitoring only %v (sorted ascending). Ports %v will NOT be monitored. "+
				"File a feature request with Datadog if you need to monitor more than %d DNS ports.",
			len(ports)+len(dropped), dnsPortsMax, ports, dropped, dnsPortsMax,
		)
		dnsMonitorTelemetry.portsTruncated.Inc()
	}
	log.Infof("DNS monitoring ports: %v", ports)

	constantEditors := make([]manager.ConstantEditor, 0, dnsPortsMax+1)
	if e.cfg.CollectDNSStats {
		constantEditors = append(constantEditors, manager.ConstantEditor{
			Name:  "dns_stats_enabled",
			Value: uint64(1),
		})
	}
	for i := 0; i < dnsPortsMax; i++ {
		var val uint64
		if i < len(ports) {
			val = uint64(uint16(ports[i]))
		}
		constantEditors = append(constantEditors, manager.ConstantEditor{
			Name:  fmt.Sprintf("dns_port_%d", i),
			Value: val,
		})
	}

	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if e.cfg.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}
	err := e.InitWithOptions(e.bytecode, manager.Options{
		RemoveRlimit: true,
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probes.SocketDNSFilter,
					UID:          probeUID,
				},
			},
		},
		ConstantEditors:           constantEditors,
		DefaultKprobeAttachMethod: kprobeAttachMethod,
		BypassEnabled:             e.cfg.BypassEnabled,
	})
	if err == nil {
		ddebpf.AddNameMappings(e.Manager, "npm_dns")
	}
	return err
}
