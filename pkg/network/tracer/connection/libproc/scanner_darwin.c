// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#include "scanner_darwin.h"

#include <arpa/inet.h>
#include <libproc.h>
#include <netinet/in.h>
#include <stdlib.h>
#include <string.h>
#include <sys/proc_info.h>
#include <sys/socket.h>

// dd_extract_socket copies a TCP or UDP IPv4/IPv6 socket into observation.
// Returns 1 when the descriptor is an inet socket we can report, otherwise 0.
static int dd_extract_socket(const struct socket_fdinfo *socket,
			     struct dd_socket_observation *observation)
{
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

// dd_scan_sockets walks host processes and socket FDs up to the given bounds
// and writes observations. Returns 0 on success and -1 if allocation or
// proc_listallpids fails. Sets *truncated when a bound stops the walk. Drops
// a process's observations if its start time changes mid-scan (PID reuse).
int dd_scan_sockets(int max_pids, int max_fds_per_pid, int max_observations,
		    struct dd_socket_observation *observations,
		    int *observation_count, int *truncated)
{
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

	struct proc_fdinfo *fds = calloc((size_t)max_fds_per_pid, sizeof(*fds));
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
		int process_bytes = proc_pidinfo(pids[i], PROC_PIDTBSDINFO, 0, &process,
						 (int)sizeof(process));
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
			int socket_bytes = proc_pidfdinfo(pids[i], fds[j].proc_fd,
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
		int verified_bytes = proc_pidinfo(pids[i], PROC_PIDTBSDINFO, 0, &verified_process,
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
