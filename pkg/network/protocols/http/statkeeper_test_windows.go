// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency time.Duration) Transaction {
	var tx WinHttpTransaction

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := uint64(latency)
	tx.Txn.RequestStarted = 1
	tx.Txn.RequestMethod = 1
	tx.Txn.ResponseLastSeen = tx.Txn.RequestStarted + latencyNS
	tx.Txn.ResponseStatusCode = uint16(code)
	tx.RequestFragment = []byte(reqFragment)

	copy(tx.Txn.Tup.RemoteAddr[:], source.AsSlice())
	tx.Txn.Tup.RemotePort = uint16(sourcePort)

	copy(tx.Txn.Tup.LocalAddr[:], dest.AsSlice())
	tx.Txn.Tup.LocalPort = uint16(destPort)

	return &tx
}
