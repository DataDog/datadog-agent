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
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
)

// NativeScanner reads socket ownership directly through Darwin libproc.
type NativeScanner struct {
	limits Limits
}

// NewNativeScanner creates a bounded host-wide libproc scanner.
func NewNativeScanner(limits Limits) (*NativeScanner, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &NativeScanner{limits: limits}, nil
}

// Scan returns a bounded point-in-time snapshot.
func (s *NativeScanner) Scan() (Snapshot, error) {
	raw := make([]C.struct_dd_socket_observation, s.limits.MaxObservations)
	var count C.int
	var truncated C.int
	result := C.dd_scan_sockets(
		C.int(s.limits.MaxPIDs),
		C.int(s.limits.MaxFDsPerPID),
		C.int(s.limits.MaxObservations),
		(*C.struct_dd_socket_observation)(unsafe.Pointer(&raw[0])),
		&count,
		&truncated,
	)
	if result != 0 {
		return Snapshot{}, fmt.Errorf("libproc socket scan failed with status %d", int(result))
	}

	snapshot := Snapshot{
		Observations: make([]Observation, 0, int(count)),
		Truncated:    truncated != 0,
	}
	for index := 0; index < int(count); index++ {
		observation, ok := convertObservation(&raw[index])
		if ok {
			snapshot.Observations = append(snapshot.Observations, observation)
		}
	}
	return snapshot, nil
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
