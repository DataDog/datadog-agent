// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

func (tx *EbpfKafkaTx) ConnTuple() KeyTuple {
	return KeyTuple{
		SrcIPHigh: tx.Tup.Saddr_h,
		SrcIPLow:  tx.Tup.Saddr_l,
		DstIPHigh: tx.Tup.Daddr_h,
		DstIPLow:  tx.Tup.Daddr_l,
		SrcPort:   tx.Tup.Sport,
		DstPort:   tx.Tup.Dport,
	}
}

func (tx *EbpfKafkaTx) TopicName() string {
	topicNameAsByteArray := make([]byte, 0, len(tx.Topic_name))
	for _, integer := range tx.Topic_name {
		if integer == 0 {
			break
		}
		topicNameAsByteArray = append(topicNameAsByteArray, byte(integer))
	}
	return string(topicNameAsByteArray)
}

func (tx *EbpfKafkaTx) APIKey() uint16 {
	return tx.Request_api_key
}

func (tx *EbpfKafkaTx) APIVersion() uint16 {
	return tx.Request_api_version
}
