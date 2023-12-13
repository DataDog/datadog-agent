// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package connection

import (
	"strings"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/davecgh/go-spew/spew"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func dumpMapsHandler(manager *manager.Manager, mapName string, currentMap *ebpf.Map) string {
	var output strings.Builder

	switch mapName {

	case "connectsock_ipv6": // maps/connectsock_ipv6 (BPF_MAP_TYPE_HASH), key C.__u64, value uintptr // C.void*
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'uintptr // C.void*'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uintptr // C.void*
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.TracerStatusMap: // maps/tracer_status (BPF_MAP_TYPE_HASH), key C.__u64, value tracerStatus
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'tracerStatus'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value offsetguess.TracerStatus
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.ConntrackStatusMap: // maps/conntrack_status (BPF_MAP_TYPE_HASH), key C.__u64, value conntrackStatus
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'conntrackStatus'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value offsetguess.ConntrackStatus
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.ConntrackMap: // maps/conntrack (BPF_MAP_TYPE_HASH), key ConnTuple, value ConnTuple
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'ConnTuple'\n")
		iter := currentMap.Iterate()
		var key ddebpf.ConnTuple
		var value ddebpf.ConnTuple
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.ConntrackTelemetryMap: // maps/conntrack_telemetry (BPF_MAP_TYPE_ARRAY), key C.u32, value conntrackTelemetry
		output.WriteString("Map: '" + mapName + "', key: 'C.u32', value: 'conntrackTelemetry'\n")
		var zero uint64
		telemetry := &ddebpf.ConntrackTelemetry{}
		if err := currentMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
			log.Tracef("error retrieving the contrack telemetry struct: %s", err)
		}
		output.WriteString(spew.Sdump(telemetry))

	case probes.SockFDLookupArgsMap: // maps/sockfd_lookup_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.SockByPidFDMap: // maps/sock_by_pid_fd (BPF_MAP_TYPE_HASH), key C.pid_fd_t, value uintptr // C.struct sock*
		output.WriteString("Map: '" + mapName + "', key: 'C.pid_fd_t', value: 'uintptr // C.struct sock*'\n")
		iter := currentMap.Iterate()
		var key ddebpf.PIDFD
		var value uintptr // C.struct sock*
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.PidFDBySockMap: // maps/pid_fd_by_sock (BPF_MAP_TYPE_HASH), key uintptr // C.struct sock*, value C.pid_fd_t
		output.WriteString("Map: '" + mapName + "', key: 'uintptr // C.struct sock*', value: 'C.pid_fd_t'\n")
		iter := currentMap.Iterate()
		var key uintptr // C.struct sock*
		var value ddebpf.PIDFD
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.ConnMap: // maps/conn_stats (BPF_MAP_TYPE_HASH), key ConnTuple, value ConnStatsWithTimestamp
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'ConnStatsWithTimestamp'\n")
		iter := currentMap.Iterate()
		var key ddebpf.ConnTuple
		var value ddebpf.ConnStats
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.TCPStatsMap: // maps/tcp_stats (BPF_MAP_TYPE_HASH), key ConnTuple, value TCPStats
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'TCPStats'\n")
		iter := currentMap.Iterate()
		var key ddebpf.ConnTuple
		var value ddebpf.TCPStats
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.ConnCloseBatchMap: // maps/conn_close_batch (BPF_MAP_TYPE_HASH), key C.__u32, value batch
		output.WriteString("Map: '" + mapName + "', key: 'C.__u32', value: 'batch'\n")
		iter := currentMap.Iterate()
		var key uint32
		var value ddebpf.Batch
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "udp_recv_sock": // maps/udp_recv_sock (BPF_MAP_TYPE_HASH), key C.__u64, value C.udp_recv_sock_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.udp_recv_sock_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ddebpf.UDPRecvSock
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "udpv6_recv_sock": // maps/udpv6_recv_sock (BPF_MAP_TYPE_HASH), key C.__u64, value C.udp_recv_sock_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.udp_recv_sock_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ddebpf.UDPRecvSock
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.PortBindingsMap: // maps/port_bindings (BPF_MAP_TYPE_HASH), key portBindingTuple, value C.__u8
		output.WriteString("Map: '" + mapName + "', key: 'portBindingTuple', value: 'C.__u8'\n")
		iter := currentMap.Iterate()
		var key ddebpf.PortBinding
		var value uint8
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.UDPPortBindingsMap: // maps/udp_port_bindings (BPF_MAP_TYPE_HASH), key portBindingTuple, value C.__u8
		output.WriteString("Map: '" + mapName + "', key: 'portBindingTuple', value: 'C.__u8'\n")
		iter := currentMap.Iterate()
		var key ddebpf.PortBinding
		var value uint8
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "pending_bind": // maps/pending_bind (BPF_MAP_TYPE_HASH), key C.__u64, value C.bind_syscall_args_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.bind_syscall_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ddebpf.BindSyscallArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case probes.TelemetryMap: // maps/telemetry (BPF_MAP_TYPE_ARRAY), key C.u32, value kernelTelemetry
		output.WriteString("Map: '" + mapName + "', key: 'C.u32', value: 'kernelTelemetry'\n")
		var zero uint64
		telemetry := &ddebpf.Telemetry{}
		if err := currentMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
			// This can happen if we haven't initialized the telemetry object yet
			// so let's just use a trace log
			log.Tracef("error retrieving the telemetry struct: %s", err)
		}
		output.WriteString(spew.Sdump(telemetry))
	}

	return output.String()
}
