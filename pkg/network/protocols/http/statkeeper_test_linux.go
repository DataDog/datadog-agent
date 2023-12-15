// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency time.Duration) Transaction {
	var event EbpfEvent

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := uint64(uint64(latency))
	event.Http.Request_started = 1
	event.Http.Request_method = 1
	event.Http.Response_last_seen = event.Http.Request_started + latencyNS
	event.Http.Response_status_code = uint16(code)
	event.Http.Request_fragment = requestFragment([]byte(reqFragment))
	event.Tuple.Saddr_l = uint64(binary.LittleEndian.Uint32(source.Bytes()))
	event.Tuple.Sport = uint16(sourcePort)
	event.Tuple.Daddr_l = uint64(binary.LittleEndian.Uint32(dest.Bytes()))
	event.Tuple.Dport = uint16(destPort)
	event.Tuple.Metadata = 1

	return &event
}
