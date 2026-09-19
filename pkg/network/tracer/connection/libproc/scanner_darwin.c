// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#include "scanner_darwin.h"

#include <arpa/inet.h>
#include <libproc.h>
#include <netinet/in.h>
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

// dd_scan_process walks one PID's socket FDs. Returns 1 when the observation
// cap is hit, 0 otherwise. Dead or unreadable PIDs leave the observation
// count unchanged (empty success). Sets *fd_truncated when MaxFDsPerPID stops
// the FD list. Drops the process's observations if its start time changes
// mid-scan (PID reuse).
static int dd_scan_process(pid_t pid, struct proc_fdinfo *fds, int max_fds_per_pid,
			   struct dd_socket_observation *observations,
			   int max_observations, int *observation_count,
			   int *fd_truncated)
{
	*fd_truncated = 0;
	if (pid <= 0) {
		return 0;
	}
	struct proc_bsdinfo process;
	memset(&process, 0, sizeof(process));
	int process_bytes = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &process,
					 (int)sizeof(process));
	if (process_bytes != (int)sizeof(process)) {
		return 0;
	}
	int fd_bytes = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds,
				    max_fds_per_pid * (int)sizeof(*fds));
	if (fd_bytes <= 0) {
		return 0;
	}
	int fd_count = fd_bytes / (int)sizeof(*fds);
	if (fd_count >= max_fds_per_pid) {
		fd_count = max_fds_per_pid;
		*fd_truncated = 1;
	}
	int process_observation_start = *observation_count;
	int full = 0;
	for (int j = 0; j < fd_count; j++) {
		if (fds[j].proc_fdtype != PROX_FDTYPE_SOCKET) {
			continue;
		}
		struct socket_fdinfo socket;
		memset(&socket, 0, sizeof(socket));
		int socket_bytes = proc_pidfdinfo(pid, fds[j].proc_fd,
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
			full = 1;
			break;
		}
		observation.pid = (uint32_t)pid;
		observation.start_sec = process.pbi_start_tvsec;
		observation.start_usec = (uint32_t)process.pbi_start_tvusec;
		observations[*observation_count] = observation;
		(*observation_count)++;
	}
	struct proc_bsdinfo verified_process;
	memset(&verified_process, 0, sizeof(verified_process));
	int verified_bytes = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &verified_process,
					  (int)sizeof(verified_process));
	if (verified_bytes != (int)sizeof(verified_process) ||
	    verified_process.pbi_start_tvsec != process.pbi_start_tvsec ||
	    verified_process.pbi_start_tvusec != process.pbi_start_tvusec) {
		*observation_count = process_observation_start;
	}
	return full;
}

int dd_scan_sockets(int max_pids, int max_fds_per_pid, int max_observations,
		    struct dd_socket_observation *observations,
		    int *observation_count, int *host_wide_truncated,
		    uint32_t *fd_truncated_pids, int fd_truncated_cap,
		    int *fd_truncated_count, int *observation_cap_hit,
		    pid_t *pids, struct proc_fdinfo *fds)
{
	if (pids == NULL || fds == NULL || observations == NULL ||
	    observation_count == NULL || host_wide_truncated == NULL ||
	    fd_truncated_pids == NULL || fd_truncated_count == NULL ||
	    observation_cap_hit == NULL) {
		return -1;
	}
	*observation_count = 0;
	*host_wide_truncated = 0;
	*fd_truncated_count = 0;
	*observation_cap_hit = 0;
	int pid_count = proc_listallpids(pids, max_pids * (int)sizeof(*pids));
	if (pid_count < 0) {
		return -1;
	}
	if (pid_count >= max_pids) {
		pid_count = max_pids;
		*host_wide_truncated = 1;
	}

	for (int i = 0; i < pid_count; i++) {
		int fd_truncated = 0;
		int full = dd_scan_process(pids[i], fds, max_fds_per_pid, observations,
					   max_observations, observation_count, &fd_truncated);
		if (fd_truncated && *fd_truncated_count < fd_truncated_cap) {
			fd_truncated_pids[*fd_truncated_count] = (uint32_t)pids[i];
			(*fd_truncated_count)++;
		}
		if (full) {
			*host_wide_truncated = 1;
			*observation_cap_hit = 1;
			break;
		}
	}
	return 0;
}

int dd_scan_pid(int pid, int max_fds_per_pid, int max_observations,
		struct dd_socket_observation *observations,
		int *observation_count, int *fd_truncated, struct proc_fdinfo *fds)
{
	*observation_count = 0;
	*fd_truncated = 0;
	if (fds == NULL || observations == NULL) {
		return -1;
	}
	(void)dd_scan_process((pid_t)pid, fds, max_fds_per_pid, observations,
			      max_observations, observation_count, fd_truncated);
	return 0;
}
