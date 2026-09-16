// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && cgo

package libproc

/*
#cgo LDFLAGS: -lproc
#include "scanner_darwin.h"
*/
import "C"

import (
	"fmt"
	"net/netip"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const initialHostObservations = 4096

// NativeScanner reads socket ownership directly through Darwin libproc.
// It is not safe for concurrent or reentrant use: Scan and ScanPID reuse
// persistent observation, PID, and FD buffers.
type NativeScanner struct {
	limits      Limits
	mu          sync.Mutex
	hostRaw     []C.struct_dd_socket_observation
	pidRaw      []C.struct_dd_socket_observation
	fdTruncated []C.uint32_t
	fdInfo      []C.struct_proc_fdinfo
	pids        []C.pid_t
}

// NewNativeScanner creates a bounded host-wide libproc scanner.
func NewNativeScanner(limits Limits) (*NativeScanner, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &NativeScanner{
		limits:      limits,
		hostRaw:     make([]C.struct_dd_socket_observation, hostObservationSeed(limits.MaxObservations)),
		pidRaw:      make([]C.struct_dd_socket_observation, limits.MaxFDsPerPID),
		fdTruncated: make([]C.uint32_t, limits.MaxPIDs),
		fdInfo:      make([]C.struct_proc_fdinfo, limits.MaxFDsPerPID),
		pids:        make([]C.pid_t, limits.MaxPIDs),
	}, nil
}

func hostObservationSeed(maxObservations int) int {
	if maxObservations < initialHostObservations {
		return maxObservations
	}
	return initialHostObservations
}

func (s *NativeScanner) growHostRaw() {
	next := len(s.hostRaw) * 2
	if next > s.limits.MaxObservations {
		next = s.limits.MaxObservations
	}
	if next <= len(s.hostRaw) {
		return
	}
	log.Debugf("darwin libproc host buffer growing from %d to %d observations (max %d)",
		len(s.hostRaw), next, s.limits.MaxObservations)
	s.hostRaw = make([]C.struct_dd_socket_observation, next)
}

// Scan returns a bounded point-in-time snapshot of the host.
//
// If the current host-buffer cap (len(hostRaw), passed to C) is hit, Scan
// grows and rescans immediately (at most four doubles: 4096 → 65536, the
// MaxObservations ceiling). Deferring growth to the next tick would set
// HostWideTruncated because the seed was small, which would spend the PID-0
// cap-3 budget on a self-inflicted condition. host_walks / scans still count
// one Scan() attempt, not these inner passes: growth is a one-time cost for
// the process lifetime, not a standing leak.
func (s *NativeScanner) Scan() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		snapshot, observationCapHit, err := s.scanHostLocked()
		if err != nil {
			return Snapshot{}, err
		}
		if !observationCapHit || len(s.hostRaw) >= s.limits.MaxObservations {
			return snapshot, nil
		}
		s.growHostRaw()
	}
}

func (s *NativeScanner) scanHostLocked() (Snapshot, bool, error) {
	var count C.int
	var hostWide C.int
	var fdCount C.int
	var observationCapHit C.int
	result := C.dd_scan_sockets(
		C.int(s.limits.MaxPIDs),
		C.int(s.limits.MaxFDsPerPID),
		C.int(len(s.hostRaw)),
		(*C.struct_dd_socket_observation)(unsafe.Pointer(&s.hostRaw[0])),
		&count,
		&hostWide,
		(*C.uint32_t)(unsafe.Pointer(&s.fdTruncated[0])),
		C.int(s.limits.MaxPIDs),
		&fdCount,
		&observationCapHit,
		(*C.pid_t)(unsafe.Pointer(&s.pids[0])),
		(*C.struct_proc_fdinfo)(unsafe.Pointer(&s.fdInfo[0])),
	)
	if result != 0 {
		return Snapshot{}, false, fmt.Errorf("libproc socket scan failed with status %d", int(result))
	}
	var fdPIDs []uint32
	if fdCount > 0 {
		fdPIDs = make([]uint32, int(fdCount))
		for i := 0; i < int(fdCount); i++ {
			fdPIDs[i] = uint32(s.fdTruncated[i])
		}
	}
	// Grow only when C hit the observation cap (full). MaxPIDs truncation
	// sets HostWideTruncated without this flag. A start-time rollback can
	// leave count < len(hostRaw) after full; the flag still grows.
	return convertSnapshot(s.hostRaw, int(count), hostWide != 0, fdPIDs), observationCapHit != 0, nil
}

// ScanPID returns a bounded snapshot of one process. A dead or unreadable PID
// is an empty successful snapshot, not an error.
func (s *NativeScanner) ScanPID(pid uint32) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count C.int
	var fdTruncated C.int
	result := C.dd_scan_pid(
		C.int(pid),
		C.int(s.limits.MaxFDsPerPID),
		C.int(s.limits.MaxFDsPerPID),
		(*C.struct_dd_socket_observation)(unsafe.Pointer(&s.pidRaw[0])),
		&count,
		&fdTruncated,
		(*C.struct_proc_fdinfo)(unsafe.Pointer(&s.fdInfo[0])),
	)
	if result != 0 {
		return Snapshot{}, fmt.Errorf("libproc pid scan failed with status %d", int(result))
	}
	var fdPIDs []uint32
	if fdTruncated != 0 {
		fdPIDs = []uint32{pid}
	}
	return convertSnapshot(s.pidRaw, int(count), false, fdPIDs), nil
}

func convertSnapshot(raw []C.struct_dd_socket_observation, count int, hostWide bool, fdTruncated []uint32) Snapshot {
	snapshot := Snapshot{
		Observations:      make([]Observation, 0, count),
		HostWideTruncated: hostWide,
		FDTruncatedPIDs:   fdTruncated,
	}
	for index := 0; index < count; index++ {
		observation, ok := convertObservation(&raw[index])
		if ok {
			snapshot.Observations = append(snapshot.Observations, observation)
		}
	}
	return snapshot
}

func convertObservation(raw *C.struct_dd_socket_observation) (Observation, bool) {
	var localBytes [16]byte
	var remoteBytes [16]byte
	for index := range localBytes {
		localBytes[index] = byte(raw.local_addr[index])
		remoteBytes[index] = byte(raw.remote_addr[index])
	}

	var local netip.Addr
	var remote netip.Addr
	var family network.ConnectionFamily
	switch uint8(raw.family) {
	case 4:
		local = netip.AddrFrom4([4]byte(localBytes[:4]))
		remote = netip.AddrFrom4([4]byte(remoteBytes[:4]))
		family = network.AFINET
	case 6:
		local = netip.AddrFrom16(localBytes)
		remote = netip.AddrFrom16(remoteBytes)
		family = network.AFINET6
	default:
		return Observation{}, false
	}
	var typ network.ConnectionType
	switch uint8(raw.protocol) {
	case 6:
		typ = network.TCP
	case 17:
		typ = network.UDP
	default:
		return Observation{}, false
	}
	start := uint64(raw.start_sec)*uint64(1e9) + uint64(raw.start_usec)*uint64(1e3)
	return Observation{
		Tuple: network.ConnectionTuple{
			Source: processutil.Address{Addr: local},
			Dest:   processutil.Address{Addr: remote},
			SPort:  uint16(raw.local_port),
			DPort:  uint16(raw.remote_port),
			Family: family,
			Type:   typ,
		},
		PID:              uint32(raw.pid),
		ProcessStartTime: start,
	}, true
}

var _ Scanner = (*NativeScanner)(nil)
