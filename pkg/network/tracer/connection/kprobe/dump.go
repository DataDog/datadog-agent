// +build linux_bpf

package kprobe

import (
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"

	"github.com/davecgh/go-spew/spew"
)

func dumpMapsHandler(managerMap *manager.Map, manager *manager.Manager) string {
	var output strings.Builder

	mapName := managerMap.Name
	currentMap, found, err := manager.GetMap(mapName)
	if err != nil || !found {
		return ""
	}

	switch mapName {

	case "connectsock_ipv6": // maps/connectsock_ipv6 (BPF_MAP_TYPE_HASH), key C.__u64, value uintptr // C.void*
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'uintptr // C.void*'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uintptr // C.void*
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.TracerStatusMap): // maps/tracer_status (BPF_MAP_TYPE_HASH), key C.__u64, value tracerStatus
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'tracerStatus'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ebpf.TracerStatus
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.ConntrackMap): // maps/conntrack (BPF_MAP_TYPE_HASH), key ConnTuple, value ConnTuple
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'ConnTuple'\n")
		iter := currentMap.Iterate()
		var key ebpf.ConnTuple
		var value ebpf.ConnTuple
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.ConntrackTelemetryMap): // maps/conntrack_telemetry (BPF_MAP_TYPE_ARRAY), key C.u32, value conntrackTelemetry
		output.WriteString("Map: '" + mapName + "', key: 'C.u32', value: 'conntrackTelemetry'\n")
		var zero uint64
		telemetry := &ebpf.ConntrackTelemetry{}
		if err := currentMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
			log.Tracef("error retrieving the contrack telemetry struct: %s", err)
		}
		output.WriteString(spew.Sdump(telemetry))

	case string(probes.SockFDLookupArgsMap): // maps/sockfd_lookup_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.SockByPidFDMap): // maps/sock_by_pid_fd (BPF_MAP_TYPE_HASH), key C.pid_fd_t, value uintptr // C.struct sock*
		output.WriteString("Map: '" + mapName + "', key: 'C.pid_fd_t', value: 'uintptr // C.struct sock*'\n")
		iter := currentMap.Iterate()
		var key ebpf.PIDFD
		var value uintptr // C.struct sock*
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.PidFDBySockMap): // maps/pid_fd_by_sock (BPF_MAP_TYPE_HASH), key uintptr // C.struct sock*, value C.pid_fd_t
		output.WriteString("Map: '" + mapName + "', key: 'uintptr // C.struct sock*', value: 'C.pid_fd_t'\n")
		iter := currentMap.Iterate()
		var key uintptr // C.struct sock*
		var value ebpf.PIDFD
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.ConnMap): // maps/conn_stats (BPF_MAP_TYPE_HASH), key ConnTuple, value ConnStatsWithTimestamp
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'ConnStatsWithTimestamp'\n")
		iter := currentMap.Iterate()
		var key ebpf.ConnTuple
		var value ebpf.ConnStats
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.TcpStatsMap): // maps/tcp_stats (BPF_MAP_TYPE_HASH), key ConnTuple, value TCPStats
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'TCPStats'\n")
		iter := currentMap.Iterate()
		var key ebpf.ConnTuple
		var value ebpf.TCPStats
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.ConnCloseBatchMap): // maps/conn_close_batch (BPF_MAP_TYPE_HASH), key C.__u32, value batch
		output.WriteString("Map: '" + mapName + "', key: 'C.__u32', value: 'batch'\n")
		iter := currentMap.Iterate()
		var key uint32
		var value ebpf.Batch
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "udp_recv_sock": // maps/udp_recv_sock (BPF_MAP_TYPE_HASH), key C.__u64, value C.udp_recv_sock_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.udp_recv_sock_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ebpf.UDPRecvSock
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.PortBindingsMap): // maps/port_bindings (BPF_MAP_TYPE_HASH), key portBindingTuple, value C.__u8
		output.WriteString("Map: '" + mapName + "', key: 'portBindingTuple', value: 'C.__u8'\n")
		iter := currentMap.Iterate()
		var key ebpf.PortBinding
		var value uint8
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.UdpPortBindingsMap): // maps/udp_port_bindings (BPF_MAP_TYPE_HASH), key portBindingTuple, value C.__u8
		output.WriteString("Map: '" + mapName + "', key: 'portBindingTuple', value: 'C.__u8'\n")
		iter := currentMap.Iterate()
		var key ebpf.PortBinding
		var value uint8
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "pending_bind": // maps/pending_bind (BPF_MAP_TYPE_HASH), key C.__u64, value C.bind_syscall_args_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.bind_syscall_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ebpf.BindSyscallArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case string(probes.TelemetryMap): // maps/telemetry (BPF_MAP_TYPE_ARRAY), key C.u32, value kernelTelemetry
		output.WriteString("Map: '" + mapName + "', key: 'C.u32', value: 'kernelTelemetry'\n")
		var zero uint64
		telemetry := &ebpf.Telemetry{}
		if err := currentMap.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(telemetry)); err != nil {
			// This can happen if we haven't initialized the telemetry object yet
			// so let's just use a trace log
			log.Tracef("error retrieving the telemetry struct: %s", err)
		}
		output.WriteString(spew.Sdump(telemetry))

	case string(probes.DoSendfileArgsMap): // maps/do_sendfile_args (BPF_MAP_TYPE_HASH), key C.__u64, value uintptr // C.struct sock*
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'uintptr // C.struct sock*'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uintptr // C.struct sock*
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	}

	return output.String()
}

func setupDumpHandler(manager *manager.Manager) {
	for _, m := range manager.Maps {
		m.DumpHandler = dumpMapsHandler
	}
}
