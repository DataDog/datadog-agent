// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#ifndef PKG_NETWORK_TRACER_CONNECTION_LIBPROC_SCANNER_DARWIN_H
#define PKG_NETWORK_TRACER_CONNECTION_LIBPROC_SCANNER_DARWIN_H

#include <stdint.h>

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

int dd_scan_sockets(int max_pids, int max_fds_per_pid, int max_observations,
		    struct dd_socket_observation *observations,
		    int *observation_count, int *truncated);

#endif
