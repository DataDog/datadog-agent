// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && cgo

package libproc

/*
#cgo LDFLAGS: -lproc
#include <arpa/inet.h>
#include <libproc.h>
#include <netinet/in.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/proc_info.h>
#include <sys/socket.h>

struct dd_socket_observation {
	uint8_t family;
	uint8_t protocol;
	uint8_t local_addr[16];
	uint8_t remote_addr[16];
	uint16_t local_port;
	uint16_t remote_port;
	uint32_t pid;
	uint64_t start_sec;
	uint32_t start_usec;
};

static int dd_extract_socket(const struct socket_fdinfo *socket,
							 struct dd_socket_observation *observation) {
	const struct socket_info *info = &socket->psi;
	const struct in_sockinfo *inet_info;
	if (info->soi_protocol == IPPROTO_TCP && info->soi_kind == SOCKINFO_TCP) {
		inet_info = &info->soi_proto.pri_tcp.tcpsi_ini;
	} else if (info->soi_protocol == IPPROTO_UDP && info->soi_kind == SOCKINFO_IN) {
		inet_info = &info->soi_proto.pri_in;
	} else {
		return 0;
	}
	if (info->soi_family != AF_INET && info->soi_family != AF_INET6) {
		return 0;
	}

	memset(observation, 0, sizeof(*observation));
	observation->family = info->soi_family == AF_INET ? 4 : 6;
	observation->protocol = (uint8_t)info->soi_protocol;
	observation->local_port = ntohs((uint16_t)inet_info->insi_lport);
	observation->remote_port = ntohs((uint16_t)inet_info->insi_fport);
	if (info->soi_family == AF_INET) {
		memcpy(observation->local_addr, &inet_info->insi_laddr.ina_46.i46a_addr4, 4);
		memcpy(observation->remote_addr, &inet_info->insi_faddr.ina_46.i46a_addr4, 4);
	} else {
		memcpy(observation->local_addr, &inet_info->insi_laddr.ina_6, 16);
		memcpy(observation->remote_addr, &inet_info->insi_faddr.ina_6, 16);
	}
	return 1;
}

static int dd_scan_sockets(int max_pids, int max_fds_per_pid,
						   int max_observations,
						   struct dd_socket_observation *observations,
						   int *observation_count, int *truncated) {
	*observation_count = 0;
	*truncated = 0;
	pid_t *pids = calloc((size_t)max_pids, sizeof(*pids));
	if (pids == NULL) {
		return -1;
	}
	int pid_count = proc_listallpids(pids, max_pids * (int)sizeof(*pids));
	if (pid_count < 0) {
		free(pids);
		return -1;
	}
	if (pid_count >= max_pids) {
		pid_count = max_pids;
		*truncated = 1;
	}

	struct proc_fdinfo *fds =
		calloc((size_t)max_fds_per_pid, sizeof(*fds));
	if (fds == NULL) {
		free(pids);
		return -1;
	}
	int full = 0;
	for (int i = 0; i < pid_count; i++) {
		if (pids[i] <= 0) {
			continue;
		}
		struct proc_bsdinfo process;
		memset(&process, 0, sizeof(process));
		int process_bytes = proc_pidinfo(pids[i], PROC_PIDTBSDINFO, 0,
									   &process, (int)sizeof(process));
		if (process_bytes != (int)sizeof(process)) {
			continue;
		}
		int fd_bytes = proc_pidinfo(pids[i], PROC_PIDLISTFDS, 0, fds,
								  max_fds_per_pid * (int)sizeof(*fds));
		if (fd_bytes <= 0) {
			continue;
		}
		int fd_count = fd_bytes / (int)sizeof(*fds);
		if (fd_count >= max_fds_per_pid) {
			fd_count = max_fds_per_pid;
			*truncated = 1;
		}
		int process_observation_start = *observation_count;
		for (int j = 0; j < fd_count; j++) {
			if (fds[j].proc_fdtype != PROX_FDTYPE_SOCKET) {
				continue;
			}
			struct socket_fdinfo socket;
			memset(&socket, 0, sizeof(socket));
			int socket_bytes =
				proc_pidfdinfo(pids[i], fds[j].proc_fd,
							   PROC_PIDFDSOCKETINFO, &socket,
							   (int)sizeof(socket));
			if (socket_bytes != (int)sizeof(socket)) {
				continue;
			}
			struct dd_socket_observation observation;
			if (!dd_extract_socket(&socket, &observation)) {
				continue;
			}
			if (*observation_count >= max_observations) {
				*truncated = 1;
				full = 1;
				break;
			}
			observation.pid = (uint32_t)pids[i];
			observation.start_sec = process.pbi_start_tvsec;
			observation.start_usec = (uint32_t)process.pbi_start_tvusec;
			observations[*observation_count] = observation;
			(*observation_count)++;
		}
		struct proc_bsdinfo verified_process;
		memset(&verified_process, 0, sizeof(verified_process));
		int verified_bytes =
			proc_pidinfo(pids[i], PROC_PIDTBSDINFO, 0, &verified_process,
						 (int)sizeof(verified_process));
		if (verified_bytes != (int)sizeof(verified_process) ||
			verified_process.pbi_start_tvsec != process.pbi_start_tvsec ||
			verified_process.pbi_start_tvusec != process.pbi_start_tvusec) {
			*observation_count = process_observation_start;
		}
		if (full) {
			break;
		}
	}
	free(fds);
	free(pids);
	return 0;
}
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
