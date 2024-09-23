// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"fmt"

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

// String returns a string representation of the kafka eBPF telemetry.
func (t *RawKernelTelemetry) String() string {
	return fmt.Sprintf(`
RawKernelTelemetry{
	"topic name size distribution": {
		"in range [1, 10]": %d,
		"in range [11, 20]": %d,
		"in range [21, 30]": %d,
		"in range [31, 40]": %d,
		"in range [41, 50]": %d,
		"in range [51, 60]": %d,
		"in range [61, 70]": %d,
		"in range [71, 80]": %d,
		"in range [81, 90]": %d,
		"in range [91, 255]": %d,
	}
	"produce no required acks": %d,
}`, t.Topic_name_size_buckets[0], t.Topic_name_size_buckets[1], t.Topic_name_size_buckets[2], t.Topic_name_size_buckets[3],
		t.Topic_name_size_buckets[4], t.Topic_name_size_buckets[5], t.Topic_name_size_buckets[6], t.Topic_name_size_buckets[7],
		t.Topic_name_size_buckets[8], t.Topic_name_size_buckets[9], t.Produce_no_required_acks)
}
