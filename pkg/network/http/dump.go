// +build linux_bpf

package http

import (
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
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
	case httpInFlightMap: // maps/http_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value httpTX
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'httpTX'\n")
		iter := currentMap.Iterate()
		var key ebpf.ConnTuple
		var value httpTX
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case httpBatchesMap: // maps/http_batches (BPF_MAP_TYPE_HASH), key httpBatchKey, value httpBatch
		output.WriteString("Map: '" + mapName + "', key: 'httpBatchKey', value: 'httpBatch'\n")
		iter := currentMap.Iterate()
		var key httpBatchKey
		var value httpBatch
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case httpBatchStateMap: // maps/http_batch_state (BPF_MAP_TYPE_HASH), key C.__u32, value C.http_batch_state_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u32', value: 'C.http_batch_state_t'\n")
		iter := currentMap.Iterate()
		var key uint32
		var value ebpf.HTTPBatchState
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case sslSockByCtxMap: // maps/ssl_sock_by_ctx (BPF_MAP_TYPE_HASH), key uintptr // C.void *, value C.ssl_sock_t
		output.WriteString("Map: '" + mapName + "', key: 'uintptr // C.void *', value: 'C.ssl_sock_t'\n")
		iter := currentMap.Iterate()
		var key uintptr // C.void *
		var value ebpf.SSLSock
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "ssl_read_args": // maps/ssl_read_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.ssl_read_args_t
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.ssl_read_args_t'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value ebpf.SSLReadArgs
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "bio_new_socket_args": // maps/bio_new_socket_args (BPF_MAP_TYPE_HASH), key C.__u64, value C.__u32
		output.WriteString("Map: '" + mapName + "', key: 'C.__u64', value: 'C.__u32'\n")
		iter := currentMap.Iterate()
		var key uint64
		var value uint32
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}

	case "fd_by_ssl_bio": // maps/fd_by_ssl_bio (BPF_MAP_TYPE_HASH), key C.__u32, value uintptr // C.void *
		output.WriteString("Map: '" + mapName + "', key: 'C.__u32', value: 'uintptr // C.void *'\n")
		iter := currentMap.Iterate()
		var key uint32
		var value uintptr // C.void *
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
