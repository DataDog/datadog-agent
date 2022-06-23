// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency time.Duration) httpTX {
	var tx ebpfHttpTx

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := C.ulonglong(uint64(latency))
	tx.request_started = 1
	tx.request_method = 1
	tx.response_last_seen = tx.request_started + latencyNS
	tx.response_status_code = _Ctype_ushort(code)
	tx.request_fragment = requestFragment([]byte(reqFragment))
	tx.tup.saddr_l = _Ctype_ulonglong(binary.LittleEndian.Uint32(source.Bytes()))
	tx.tup.sport = _Ctype_ushort(sourcePort)
	tx.tup.daddr_l = _Ctype_ulonglong(binary.LittleEndian.Uint32(dest.Bytes()))
	tx.tup.dport = _Ctype_ushort(destPort)
	tx.tup.metadata = 1

	return tx
}
