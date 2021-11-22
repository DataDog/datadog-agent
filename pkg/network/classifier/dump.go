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

	case tlsInFlightMap: // maps/tls_in_flight (BPF_MAP_TYPE_HASH), key ConnTuple, value tlsSession
		output.WriteString("Map: '" + mapName + "', key: 'ConnTuple', value: 'tlsSession'\n")
		iter := currentMap.Iterate()
		var key ebpf.ConnTuple
		var value tlsSession
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
