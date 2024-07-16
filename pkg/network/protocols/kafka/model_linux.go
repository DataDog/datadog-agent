// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// ConnTuple returns the connection tuple for the transaction
func (tx *EbpfTx) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: tx.Tup.Saddr_h,
		SrcIPLow:  tx.Tup.Saddr_l,
		DstIPHigh: tx.Tup.Daddr_h,
		DstIPLow:  tx.Tup.Daddr_l,
		SrcPort:   tx.Tup.Sport,
		DstPort:   tx.Tup.Dport,
	}
}

// APIKey returns the API key for the transaction
func (tx *EbpfTx) APIKey() uint16 {
	return uint16(tx.Transaction.Request_api_key)
}

// APIVersion returns the API version for the transaction
func (tx *EbpfTx) APIVersion() uint16 {
	return uint16(tx.Transaction.Request_api_version)
}

// RecordsCount returns the number of records in the transaction
func (tx *EbpfTx) RecordsCount() uint32 {
	return tx.Transaction.Records_count
}

// ErrorCode returns the error code in the transaction
func (tx *EbpfTx) ErrorCode() int8 {
	return tx.Transaction.Error_code
}

// RequestLatency returns the latency of the request in nanoseconds
func (tx *EbpfTx) RequestLatency() float64 {
	if uint64(tx.Transaction.Request_started) == 0 || uint64(tx.Transaction.Response_last_seen) == 0 {
		return 0
	}
	return protocols.NSTimestampToFloat(tx.Transaction.Response_last_seen - tx.Transaction.Request_started)
}
