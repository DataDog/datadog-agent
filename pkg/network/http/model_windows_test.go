// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

func TestLatency(t *testing.T) {
	tx := httpTX{
		ResponseLastSeen: 2e6,
		RequestStarted:   1e6,
	}
	// quantization brings it down
	assert.Equal(t, 999424.0, tx.RequestLatency())
}

func (tx *httpTX) SetMethodUnknown() {
	tx.RequestMethod = 0
}

func generateTransactionWithFragment(fragment string) httpTX {
	var tx httpTX

	for i := 0; i < len(tx.RequestFragment) && i < len(fragment); i++ {
		tx.Txn.RequestFragment[i] = uint8(fragment[i])
	}
	return tx
}

func generateIPv4HTTPTransaction(client util.Address, server util.Address, cliPort int, srvPort int, path string, code int, latency time.Duration) httpTX {
	var tx httpTX

	reqFragment := fmt.Sprintf("GET %s HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0", path)
	latencyNS := uint64(uint64(latency))
	cli := client.Bytes()
	srv := server.Bytes()

	tx.Tnx.RequestMethod = 1
	tx.Txn.RequestStarted = 1
	tx.Txn.ResponseLastSeen = tx.Txn.RequestStarted + latencyNS
	tx.Txn.ResponseStatusCode = uint16(code)
	for i := 0; i < len(tx.Txn.RequestFragment) && i < len(reqFragment); i++ {
		tx.Txn.RequestFragment[i] = uint8(reqFragment[i])
	}
	for i := 0; i < len(tx.Txn.Tup.CliAddr) && i < len(cli); i++ {
		tx.Txn.Tup.CliAddr[i] = cli[i]
	}
	for i := 0; i < len(tx.Txn.Tup.SrvAddr) && i < len(srv); i++ {
		tx.Txn.Tup.SrvAddr[i] = srv[i]
	}
	tx.Txn.Tup.CliPort = uint16(cliPort)
	tx.Txn.Tup.SrvPort = uint16(srvPort)

	return tx
}
