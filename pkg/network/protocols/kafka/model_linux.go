// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import "github.com/DataDog/datadog-agent/pkg/network/types"

func (tx *EbpfKafkaTx) ConnTuple() types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPHigh: tx.Tup.Saddr_h,
		SrcIPLow:  tx.Tup.Saddr_l,
		DstIPHigh: tx.Tup.Daddr_h,
		DstIPLow:  tx.Tup.Daddr_l,
		SrcPort:   tx.Tup.Sport,
		DstPort:   tx.Tup.Dport,
	}
}

func (tx *EbpfKafkaTx) TopicName() string {
	return string(tx.Topic_name[:tx.Topic_name_size])
}

func (tx *EbpfKafkaTx) APIKey() uint16 {
	return tx.Request_api_key
}

func (tx *EbpfKafkaTx) APIVersion() uint16 {
	return tx.Request_api_version
}
