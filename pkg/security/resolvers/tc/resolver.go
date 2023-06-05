// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"fmt"
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/hashicorp/go-multierror"
	"github.com/vishvananda/netlink"
)

// NetDeviceKey is used to uniquely identify a network device
type NetDeviceKey struct {
	IfIndex          uint32
	NetNS            uint32
	NetworkDirection manager.TrafficType
}

type Resolver struct {
	sync.RWMutex
	config   *config.Config
	programs map[NetDeviceKey]*manager.Probe
}

func NewResolver(config *config.Config) *Resolver {
	return &Resolver{
		config:   config,
		programs: make(map[NetDeviceKey]*manager.Probe),
	}
}

func (tcr *Resolver) SendTCProgramsStats(statsdClient statsd.ClientInterface) {
	tcr.RLock()
	defer tcr.RUnlock()

	if val := float64(len(tcr.programs)); val > 0 {
		_ = statsdClient.Gauge(metrics.MetricTCProgram, val, []string{}, 1.0)
	}
}

func (tcr *Resolver) SelectTCProbes() manager.ProbesSelector {
	tcr.RLock()
	defer tcr.RUnlock()

	// Although unlikely, a race is still possible with the umount event of a network namespace:
	//   - a reload event is triggered
	//   - selectTCProbes is invoked and the list of currently running probes is generated
	//   - a container exits and the umount event of its network namespace is handled now (= its TC programs are stopped)
	//   - the manager executes UpdateActivatedProbes
	// In this setup, if we didn't use the best effort selector, the manager would try to init & attach a program that
	// was deleted when the container exited.
	var activatedProbes manager.BestEffort
	for _, tcProbe := range tcr.programs {
		if tcProbe.IsRunning() {
			activatedProbes.Selectors = append(activatedProbes.Selectors, &manager.ProbeSelector{
				ProbeIdentificationPair: tcProbe.ProbeIdentificationPair,
			})
		}
	}
	return &activatedProbes
}

// SetupNewTCClassifierWithNetNSHandle creates and attaches TC probes on the provided device. WARNING: this function
// will not close the provided netns handle, so the caller of this function needs to take care of it.
func (tcr *Resolver) SetupNewTCClassifierWithNetNSHandle(device model.NetDevice, netnsHandle *os.File, m *manager.Manager) error {
	tcr.Lock()
	defer tcr.Unlock()

	var combinedErr multierror.Error
	for _, tcProbe := range probes.GetTCProbes() {
		// make sure we're not overriding an existing network probe
		deviceKey := NetDeviceKey{IfIndex: device.IfIndex, NetNS: device.NetNS, NetworkDirection: tcProbe.NetworkDirection}
		_, ok := tcr.programs[deviceKey]
		if ok {
			continue
		}

		newProbe := tcProbe.Copy()
		newProbe.CopyProgram = true
		newProbe.UID = probes.SecurityAgentUID + device.GetKey()
		newProbe.IfIndex = int(device.IfIndex)
		newProbe.IfIndexNetns = uint64(netnsHandle.Fd())
		newProbe.IfIndexNetnsID = device.NetNS
		newProbe.KeepProgramSpec = false
		newProbe.TCFilterPrio = tcr.config.NetworkClassifierPriority
		newProbe.TCFilterHandle = netlink.MakeHandle(0, tcr.config.NetworkClassifierHandle)

		netnsEditor := []manager.ConstantEditor{
			{
				Name:  "netns",
				Value: uint64(device.NetNS),
			},
		}

		if err := m.CloneProgram(probes.SecurityAgentUID, newProbe, netnsEditor, nil); err != nil {
			_ = multierror.Append(&combinedErr, fmt.Errorf("couldn't clone %s: %v", tcProbe.ProbeIdentificationPair, err))
		} else {
			tcr.programs[deviceKey] = newProbe
		}
	}
	return combinedErr.ErrorOrNil()
}

// flushNetworkNamespace thread unsafe version of FlushNetworkNamespace
func (tcr *Resolver) FlushNetworkNamespaceID(namespaceID uint32, m *manager.Manager) {
	tcr.Lock()
	defer tcr.Unlock()

	for tcKey, tcProbe := range tcr.programs {
		if tcKey.NetNS == namespaceID {
			_ = m.DetachHook(tcProbe.ProbeIdentificationPair)
			delete(tcr.programs, tcKey)
		}
	}
}

// FlushInactiveProbes detaches and deletes inactive probes. This function returns a map containing the count of interfaces
// per network namespace (ignoring the interfaces that are lazily deleted).
func (tcr *Resolver) FlushInactiveProbes(m *manager.Manager, isLazy func(string) bool) map[uint32]int {
	tcr.Lock()
	defer tcr.Unlock()

	probesCountNoLazyDeletion := make(map[uint32]int)

	var linkName string
	for tcKey, tcProbe := range tcr.programs {
		if !tcProbe.IsTCFilterActive() {
			_ = m.DetachHook(tcProbe.ProbeIdentificationPair)
			delete(tcr.programs, tcKey)
		} else {
			link, err := tcProbe.ResolveLink()
			if err == nil {
				linkName = link.Attrs().Name
			} else {
				linkName = ""
			}
			// ignore interfaces that are lazily deleted
			if link.Attrs().HardwareAddr.String() != "" && !isLazy(linkName) {
				probesCountNoLazyDeletion[tcKey.NetNS]++
			}
		}
	}

	return probesCountNoLazyDeletion
}

func (tcr *Resolver) ResolveNetworkDeviceIfName(ifIndex, netNS uint32) (string, bool) {
	tcr.RLock()
	defer tcr.RUnlock()

	for _, direction := range []manager.TrafficType{manager.Egress, manager.Ingress} {
		key := NetDeviceKey{
			IfIndex:          ifIndex,
			NetNS:            netNS,
			NetworkDirection: direction,
		}

		tcProbe, ok := tcr.programs[key]
		if ok {
			return tcProbe.IfName, true
		}
	}
	return "", false
}
