// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package classifier

import (
	"strings"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	"github.com/davecgh/go-spew/spew"
)

func dumpMapsHandler(manager *manager.Manager, mapName string, currentMap *ebpf.Map) string {
	var output strings.Builder

	switch mapName {
	case tlsInFlightMap: // maps/tls_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value tlsSession
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'tlsSession'\n")
		iter := currentMap.Iterate()
		var key ddebpf.ConnTuple
		var value tlsSession
		for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
			output.WriteString(spew.Sdump(key, value))
		}
	}
	return output.String()
}
