// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/socket-contention-kern.c pkg/ebpf/bytecode/build/runtime/socket-contention.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/socket-contention.c pkg/ebpf/bytecode/runtime/socket-contention.go runtime

package socketcontention

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/socketcontention/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpfmaps "github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	socketContentionAsset = "socket-contention.o"
	socketContentionGroup = "socket_contention"
	socketContentionUID   = "socket_contention"

	contentionBeginTracepointPath = "events/lock/contention_begin/id"
	contentionEndTracepointPath   = "events/lock/contention_end/id"

	tstampMapName         = "tstamp"
	tstampCPUMapName      = "tstamp_cpu"
	statsMapName          = "socket_contention_stats"
	lockIdentitiesMapName = "socket_lock_identities"

	kprobeSockInitDataName      = "kprobe__sock_init_data"
	kprobeTCPConnectName        = "kprobe__tcp_connect"
	kretprobeInetCskAcceptName  = "kretprobe__inet_csk_accept"
	kprobeTCPCloseName          = "kprobe__tcp_close"
	kprobeInetCskListenStopName = "kprobe__inet_csk_listen_stop"
	kprobeSkDestructName        = "kprobe____sk_destruct"
	tpContentionBeginName       = "tp_contention_begin"
	tpContentionEndName         = "tp_contention_end"
)

var minimumKernelVersion = kernel.VersionCode(5, 5, 0)

func contentionTracepointsSupported() bool {
	traceFSRoot, err := tracefs.Root()
	if err != nil {
		return false
	}

	if _, err := os.Stat(filepath.Join(traceFSRoot, contentionBeginTracepointPath)); errors.Is(err, os.ErrNotExist) {
		return false
	}

	if _, err := os.Stat(filepath.Join(traceFSRoot, contentionEndTracepointPath)); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// Probe is the eBPF side of the socket contention check.
type Probe struct {
	mgr               *ddebpf.Manager
	statsMap          *ebpfmaps.GenericMap[ebpfSocketContentionKey, []ebpfSocketContentionStats]
	lockIdentitiesMap *ebpfmaps.GenericMap[uint64, ebpfSocketLockIdentity]
}

// NewProbe creates a [Probe].
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("detect kernel version: %w", err)
	}
	if kv < minimumKernelVersion {
		return nil, fmt.Errorf("minimum kernel version %s not met, read %s", minimumKernelVersion, kv)
	}
	if !contentionTracepointsSupported() {
		return nil, fmt.Errorf("lock contention tracepoints are not available on this kernel")
	}
	if err := ddebpf.Setup(cfg, nil); err != nil {
		return nil, fmt.Errorf("setup CO-RE loader: %w", err)
	}

	var probe *Probe
	err = ddebpf.LoadCOREAsset(socketContentionAsset, func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("load CO-RE socket contention probe: %w", err)
	}

	return probe, nil
}

func startProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*Probe, error) {
	m := ddebpf.NewManagerWithDefault(&manager.Manager{
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: kprobeSockInitDataName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: kprobeTCPConnectName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: kretprobeInetCskAcceptName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: kprobeTCPCloseName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: kprobeInetCskListenStopName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: kprobeSkDestructName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: tpContentionBeginName, UID: socketContentionUID}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: tpContentionEndName, UID: socketContentionUID}},
		},
		Maps: []*manager.Map{
			{Name: tstampMapName},
			{Name: tstampCPUMapName},
			{Name: lockIdentitiesMapName},
			{Name: statsMapName},
		},
	}, socketContentionGroup, &ebpftelemetry.ErrorsTelemetryModifier{})

	managerOptions.RemoveRlimit = true
	if managerOptions.MapSpecEditors == nil {
		managerOptions.MapSpecEditors = make(map[string]manager.MapSpecEditor)
	}
	managerOptions.MapSpecEditors[tstampMapName] = manager.MapSpecEditor{
		MaxEntries: 1024,
		EditorFlag: manager.EditMaxEntries,
	}
	managerOptions.MapSpecEditors[statsMapName] = manager.MapSpecEditor{
		MaxEntries: 4096,
		EditorFlag: manager.EditMaxEntries,
	}
	managerOptions.MapSpecEditors[lockIdentitiesMapName] = manager.MapSpecEditor{
		MaxEntries: 32768,
		EditorFlag: manager.EditMaxEntries,
	}

	if err := m.InitWithOptions(buf, &managerOptions); err != nil {
		return nil, fmt.Errorf("init socket contention manager: %w", err)
	}
	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("start socket contention manager: %w", err)
	}

	statsMap, err := ebpfmaps.GetMap[ebpfSocketContentionKey, []ebpfSocketContentionStats](m.Manager, statsMapName)
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("get stats map %q: %w", statsMapName, err)
	}
	lockIdentitiesMap, err := ebpfmaps.GetMap[uint64, ebpfSocketLockIdentity](m.Manager, lockIdentitiesMapName)
	if err != nil {
		_ = m.Stop(manager.CleanAll)
		return nil, fmt.Errorf("get lock identities map %q: %w", lockIdentitiesMapName, err)
	}

	ddebpf.AddNameMappings(m.Manager, socketContentionGroup)
	ddebpf.AddProbeFDMappings(m.Manager)

	return &Probe{
		mgr:               m,
		statsMap:          statsMap,
		lockIdentitiesMap: lockIdentitiesMap,
	}, nil
}

// Close releases all associated resources.
func (p *Probe) Close() {
	if p == nil || p.mgr == nil {
		return
	}

	ddebpf.RemoveNameMappings(p.mgr.Manager)
	if err := p.mgr.Stop(manager.CleanAll); err != nil {
		log.Warnf("error stopping socket contention manager: %s", err)
	}
}

func toObjectKind(value uint8) string {
	switch value {
	case 1:
		return "socket"
	default:
		return "unknown"
	}
}

func toSocketType(value uint16) string {
	switch value {
	case 1:
		return "stream"
	case 2:
		return "dgram"
	case 3:
		return "raw"
	case 5:
		return "seqpacket"
	default:
		return "unknown"
	}
}

func toFamily(value uint16) string {
	switch value {
	case 1:
		return "unix"
	case 2:
		return "inet"
	case 10:
		return "inet6"
	default:
		return "unknown"
	}
}

func toProtocol(value uint16) string {
	switch value {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		if value == 0 {
			return "unknown"
		}
		return fmt.Sprintf("%d", value)
	}
}

func toLockSubtype(value uint8) string {
	switch value {
	case 1:
		return "sk_lock"
	case 2:
		return "sk_wait_queue"
	case 3:
		return "callback_lock"
	case 4:
		return "error_queue_lock"
	case 5:
		return "receive_queue_lock"
	case 6:
		return "write_queue_lock"
	default:
		return "unknown"
	}
}

// GetAndFlush gets the current stats and clears the map.
func (p *Probe) GetAndFlush() model.SocketContentionStats {
	var stats model.SocketContentionStats

	iter := p.statsMap.Iterate()
	var rawKey ebpfSocketContentionKey
	var rawStats []ebpfSocketContentionStats
	var keysToDelete []ebpfSocketContentionKey

	for iter.Next(&rawKey, &rawStats) {
		if rawKey.Object_kind != 1 {
			keysToDelete = append(keysToDelete, rawKey)
			continue
		}

		entry := model.SocketContentionEntry{
			ObjectKind: toObjectKind(rawKey.Object_kind),
			SocketType: toSocketType(rawKey.Socket_type),
			Family:     toFamily(rawKey.Family),
			Protocol:   toProtocol(rawKey.Protocol),
			LockSubtype: toLockSubtype(rawKey.Lock_subtype),
			CgroupID:   rawKey.Cgroup_id,
			Flags:      rawKey.Flags,
			MinTimeNS:  math.MaxUint64,
		}

		for _, perCPUStat := range rawStats {
			entry.TotalTimeNS += perCPUStat.Total_time_ns
			entry.Count += perCPUStat.Count
			if perCPUStat.Max_time_ns > entry.MaxTimeNS {
				entry.MaxTimeNS = perCPUStat.Max_time_ns
			}
			if perCPUStat.Min_time_ns != 0 && perCPUStat.Min_time_ns < entry.MinTimeNS {
				entry.MinTimeNS = perCPUStat.Min_time_ns
			}
		}
		if entry.MinTimeNS == math.MaxUint64 {
			entry.MinTimeNS = 0
		}
		if entry.Count > 0 {
			stats = append(stats, entry)
		}
		keysToDelete = append(keysToDelete, rawKey)
	}

	if err := iter.Err(); err != nil {
		log.Warnf("error iterating socket contention stats: %s", err)
	}

	for _, key := range keysToDelete {
		deleteKey := key
		if err := p.statsMap.Delete(&deleteKey); err != nil {
			log.Warnf("failed to delete socket contention stat: %s", err)
		}
	}

	return stats
}

// DebugListLockIdentities returns the currently registered lock identities for tests.
func (p *Probe) DebugListLockIdentities() ([]model.SocketLockIdentity, error) {
	iter := p.lockIdentitiesMap.Iterate()
	var rawKey uint64
	var rawIdentity ebpfSocketLockIdentity
	var identities []model.SocketLockIdentity

	for iter.Next(&rawKey, &rawIdentity) {
		identities = append(identities, model.SocketLockIdentity{
			LockAddr:      rawKey,
			SockPtr:       rawIdentity.Sock_ptr,
			SocketCookie: rawIdentity.Socket_cookie,
			CgroupID:      rawIdentity.Cgroup_id,
			Family:        toFamily(rawIdentity.Family),
			Protocol:      toProtocol(rawIdentity.Protocol),
			SocketType:    toSocketType(rawIdentity.Socket_type),
			LockSubtype:   toLockSubtype(rawIdentity.Lock_subtype),
		})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("iterate socket lock identities: %w", err)
	}
	return identities, nil
}
