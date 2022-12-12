// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

func (tx *ebpfKafkaTx) SrcIPHigh() uint64 {
	return tx.Tup.Saddr_h
}

func (tx *ebpfKafkaTx) SrcIPLow() uint64 {
	return tx.Tup.Saddr_l
}

func (tx *ebpfKafkaTx) SrcPort() uint16 {
	return tx.Tup.Sport
}

func (tx *ebpfKafkaTx) DstIPHigh() uint64 {
	return tx.Tup.Daddr_h
}

func (tx *ebpfKafkaTx) DstIPLow() uint64 {
	return tx.Tup.Daddr_l
}

func (tx *ebpfKafkaTx) DstPort() uint16 {
	return tx.Tup.Dport
}

func (tx *ebpfKafkaTx) TopicName() string {
	topicNameAsByteArray := make([]byte, 0, len(tx.Topic_name))
	for _, integer := range tx.Topic_name {
		if integer == 0 {
			break
		}
		topicNameAsByteArray = append(topicNameAsByteArray, byte(integer))
	}
	return string(topicNameAsByteArray)
}

func (tx *ebpfKafkaTx) APIKey() uint16 {
	return tx.Request_api_key
}

// Transactions returns the slice of Kafka transactions embedded in the batch
func (batch *kafkaBatch) Transactions() []ebpfKafkaTx {
	return batch.Txs[:]
}
