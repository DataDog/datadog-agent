// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tc holds tc related files
package tc

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/hashicorp/go-multierror"
	"github.com/vishvananda/netlink"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProgramKey is used to uniquely identify a tc program
type ProgramKey struct {
	UID              string
	FuncName         string
	NetDevice        model.NetDevice
	NetworkDirection manager.TrafficType
}

// Key return an identifier
func (t *ProgramKey) Key() string {
	return t.UID + "_" + t.FuncName + "_" + t.NetDevice.GetKey()
}

// programEntry holds a TC program entry
// we keep track of the program ID in addition to the probe itself
// in case the probe is removed from the manager, or reset, to be
// able to clean up ddebpf mappings
type programEntry struct {
	programID uint32
	probe     *manager.Probe
}

// Resolver defines a TC resolver
type Resolver struct {
	sync.RWMutex
	config   *config.Config
	programs map[ProgramKey]programEntry
}

// NewResolver returns a TC resolver
func NewResolver(config *config.Config) *Resolver {
	return &Resolver{
		config:   config,
		programs: make(map[ProgramKey]programEntry),
	}
}

// SendTCProgramsStats sends TC programs stats
func (tcr *Resolver) SendTCProgramsStats(statsdClient statsd.ClientInterface) {
	tcr.RLock()
	defer tcr.RUnlock()

	if val := float64(len(tcr.programs)); val > 0 {
		_ = statsdClient.Gauge(metrics.MetricTCProgram, val, []string{}, 1.0)
	}
}

// SelectTCProbes selects TC probes
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
	for _, entry := range tcr.programs {
		if entry.probe.IsRunning() {
			activatedProbes.Selectors = append(activatedProbes.Selectors, &manager.ProbeSelector{
				ProbeIdentificationPair: entry.probe.ProbeIdentificationPair,
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
	for _, tcProbe := range probes.GetTCProbes(tcr.config.NetworkIngressEnabled, tcr.config.NetworkRawPacketEnabled) {
		// make sure we're not overriding an existing network probe
		progKey := ProgramKey{
			UID:              tcProbe.UID,
			FuncName:         tcProbe.EBPFFuncName,
			NetDevice:        device,
			NetworkDirection: tcProbe.NetworkDirection,
		}
		// skip if probe already attached
		_, ok := tcr.programs[progKey]
		if ok {
			continue
		}

		newProbe := tcProbe.Copy()
		newProbe.CopyProgram = true
		newProbe.UID = progKey.Key()

		newProbe.IfIndex = int(device.IfIndex)
		newProbe.IfIndexNetns = uint64(netnsHandle.Fd())
		newProbe.IfIndexNetnsID = device.NetNS
		newProbe.KeepProgramSpec = false
		newProbe.TCFilterPrio = tcr.config.NetworkClassifierPriority

		if slices.Contains(probes.RawPacketTCProgram, tcProbe.EBPFFuncName) {
			newProbe.TCFilterHandle = netlink.MakeHandle(0, tcr.config.RawNetworkClassifierHandle)
		} else {
			newProbe.TCFilterHandle = netlink.MakeHandle(0, tcr.config.NetworkClassifierHandle)
		}

		netnsEditor := []manager.ConstantEditor{
			{
				Name:  "netns",
				Value: uint64(device.NetNS),
			},
		}

		if err := m.CloneProgram(probes.SecurityAgentUID, newProbe, netnsEditor, nil); err != nil {
			linkNotFoundErr := &netlink.LinkNotFoundError{}
			if errors.As(err, linkNotFoundErr) {
				// return now since we won't be able to attach anything at all
				return err
			}
			_ = multierror.Append(&combinedErr, fmt.Errorf("couldn't clone %s: %w", tcProbe.ProbeIdentificationPair, err))
		} else {
			entry := programEntry{
				programID: newProbe.ID(),
				probe:     newProbe,
			}

			tcr.programs[progKey] = entry

			// do not use dynamic program name here, it explodes cardinality
			ddebpf.AddProgramNameMapping(entry.programID, entry.probe.EBPFFuncName, "cws")
		}
	}
	return combinedErr.ErrorOrNil()
}

// detachHook detaches and deletes a TC hook from the resolver, needs to be called with the lock held
func (tcr *Resolver) detachHook(tcKey ProgramKey, entry programEntry, m *manager.Manager) {
	ddebpf.RemoveProgramID(entry.programID, "cws")
	delete(tcr.programs, tcKey)

	_ = m.DetachHook(entry.probe.ProbeIdentificationPair)
}

// FlushNetworkNamespaceID flushes network ID
func (tcr *Resolver) FlushNetworkNamespaceID(namespaceID uint32, m *manager.Manager) {
	tcr.Lock()
	defer tcr.Unlock()

	for tcKey, entry := range tcr.programs {
		if tcKey.NetDevice.NetNS == namespaceID {
			tcr.detachHook(tcKey, entry, m)
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
	for tcKey, entry := range tcr.programs {
		if !entry.probe.IsTCFilterActive() {
			tcr.detachHook(tcKey, entry, m)
		} else {
			link, err := entry.probe.ResolveLink()
			if err == nil {
				linkName = link.Attrs().Name
			} else {
				linkName = ""
			}
			// ignore interfaces that are lazily deleted
			if link.Attrs().HardwareAddr.String() != "" && !isLazy(linkName) {
				probesCountNoLazyDeletion[tcKey.NetDevice.NetNS]++
			}
		}
	}

	return probesCountNoLazyDeletion
}

// ResolveNetworkDeviceIfName resolves network device name
func (tcr *Resolver) ResolveNetworkDeviceIfName(ifIndex, netNS uint32) (string, bool) {
	tcr.RLock()
	defer tcr.RUnlock()

	for key := range tcr.programs {
		if key.NetDevice.IfIndex == ifIndex && key.NetDevice.NetNS == netNS {
			return key.NetDevice.Name, true
		}
	}

	return "", false
}
