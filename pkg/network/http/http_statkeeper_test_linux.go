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
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency time.Duration) transaction.HttpTX {
	var tx transaction.EbpfHttpTx

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := uint64(latency)
	tx.Request_started = 1
	tx.Request_method = 1
	tx.Response_last_seen = tx.Request_started + latencyNS
	tx.Response_status_code = uint16(code)
	tx.Request_fragment = transaction.RequestFragment([]byte(reqFragment))
	tx.Tup.Saddr_l = uint64(binary.LittleEndian.Uint32(source.Bytes()))
	tx.Tup.Sport = uint16(sourcePort)
	tx.Tup.Daddr_l = uint64(binary.LittleEndian.Uint32(dest.Bytes()))
	tx.Tup.Dport = uint16(destPort)
	tx.Tup.Metadata = 1

	return &tx
}
