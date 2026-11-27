// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tc

import (
	"cmp"
	"os"
	"slices"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/vishvananda/netlink"
)

// ProgramDump describes a classifier attached to a network device
type ProgramDump struct {
	FuncName   string
	Direction  string
	AttachMode string
	ProgramID  uint32
	LinkID     uint32 `json:",omitempty"`
	// Position is the rank of the classifier among the programs of its hook, 0 running first
	Position *int `json:",omitempty"`
}

type hook struct {
	ifIndex   uint32
	direction manager.TrafficType
}

type hookProgram struct {
	programID uint32
	linkID    uint32
}

// DumpNetworkNamespacePrograms returns the classifiers of the devices of a network namespace by ifindex. TCX positions
// are queried from nsHandle, since TCX resolves the ifindex in the namespace of the caller.
func (tcr *Resolver) DumpNetworkNamespacePrograms(nsID uint32, nsHandle *os.File, ntl *manager.NetlinkSocket) map[uint32][]ProgramDump {
	type programKeyEntry struct {
		key   ProgramKey
		entry programEntry
	}

	tcr.RLock()
	var entries []programKeyEntry
	for key, entry := range tcr.programs {
		if key.NetNS == nsID {
			entries = append(entries, programKeyEntry{key: key, entry: entry})
		}
	}
	tcr.RUnlock()

	hooks := make(map[manager.TCAttachMode]map[hook][]hookProgram)
	for _, e := range entries {
		if hooks[e.entry.attachMode] == nil {
			hooks[e.entry.attachMode] = make(map[hook][]hookProgram)
		}
		hooks[e.entry.attachMode][hook{ifIndex: e.key.IfIndex, direction: e.key.NetworkDirection}] = nil
	}

	if legacyHooks := hooks[manager.TCAttachModeLegacy]; ntl != nil {
		for h := range legacyHooks {
			legacyHooks[h], _ = listFilters(ntl, h)
		}
	}

	if tcxHooks := hooks[manager.TCAttachModeTCX]; len(tcxHooks) > 0 && nsHandle != nil {
		_ = RunInNetNS(nsHandle, func() error {
			for h := range tcxHooks {
				tcxHooks[h], _ = queryTCXPrograms(h)
			}
			return nil
		})
	}

	dumps := make(map[uint32][]ProgramDump)
	for _, e := range entries {
		dump := ProgramDump{
			FuncName:   e.key.FuncName,
			Direction:  e.key.NetworkDirection.String(),
			AttachMode: e.entry.attachMode.String(),
			ProgramID:  e.entry.programID,
		}

		hookPrograms := hooks[e.entry.attachMode][hook{ifIndex: e.key.IfIndex, direction: e.key.NetworkDirection}]
		if position := slices.IndexFunc(hookPrograms, func(p hookProgram) bool { return p.programID == e.entry.programID }); position >= 0 {
			dump.Position = &position
			dump.LinkID = hookPrograms[position].linkID
		}

		dumps[e.key.IfIndex] = append(dumps[e.key.IfIndex], dump)
	}

	for _, deviceDumps := range dumps {
		slices.SortFunc(deviceDumps, func(a, b ProgramDump) int {
			return cmp.Or(
				cmp.Compare(a.Direction, b.Direction),
				cmp.Compare(positionOrLast(a.Position), positionOrLast(b.Position)),
				cmp.Compare(a.FuncName, b.FuncName),
			)
		})
	}
	return dumps
}

func positionOrLast(position *int) int {
	if position == nil {
		return int(^uint(0) >> 1)
	}
	return *position
}

// listFilters returns the filters of a clsact hook, in the order in which they run
func listFilters(ntl *manager.NetlinkSocket, h hook) ([]hookProgram, error) {
	parent := uint32(netlink.HANDLE_MIN_EGRESS)
	if h.direction == manager.Ingress {
		parent = netlink.HANDLE_MIN_INGRESS
	}

	filters, err := ntl.Sock.FilterList(&netlink.Device{LinkAttrs: netlink.LinkAttrs{Index: int(h.ifIndex)}}, parent)
	if err != nil {
		return nil, err
	}

	programs := make([]hookProgram, 0, len(filters))
	for _, filter := range filters {
		var programID uint32
		if bpfFilter, ok := filter.(*netlink.BpfFilter); ok {
			programID = uint32(bpfFilter.Id)
		}
		programs = append(programs, hookProgram{programID: programID})
	}
	return programs, nil
}

// queryTCXPrograms returns the programs of a TCX hook, in the order in which they run
func queryTCXPrograms(h hook) ([]hookProgram, error) {
	tcxPrograms, err := QueryTCXPrograms(int(h.ifIndex), h.direction)
	if err != nil {
		return nil, err
	}

	programs := make([]hookProgram, 0, len(tcxPrograms))
	for _, prog := range tcxPrograms {
		programs = append(programs, hookProgram{programID: prog.ID, linkID: prog.LinkID})
	}
	return programs, nil
}
