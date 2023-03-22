// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kafka

import "github.com/DataDog/datadog-agent/pkg/process/util"

const (
	ProduceAPIKey = 0
	FetchAPIKey   = 1
)

// KeyTuple represents the network tuple for a group of Kafka transactions
type KeyTuple struct {
	SrcIPHigh uint64
	SrcIPLow  uint64

	DstIPHigh uint64
	DstIPLow  uint64

	// ports separated for alignment/size optimization
	SrcPort uint16
	DstPort uint16
}

// Key is an identifier for a group of Kafka transactions
type Key struct {
	RequestAPIKey  uint16
	RequestVersion uint16
	TopicName      string
	KeyTuple
}

// NewKey generates a new Key
func NewKey(saddr, daddr util.Address, sport, dport uint16, topicName string) Key {
	return Key{
		KeyTuple:  NewKeyTuple(saddr, daddr, sport, dport),
		TopicName: topicName,
	}
}

// NewKeyTuple generates a new KeyTuple
func NewKeyTuple(saddr, daddr util.Address, sport, dport uint16) KeyTuple {
	saddrl, saddrh := util.ToLowHigh(saddr)
	daddrl, daddrh := util.ToLowHigh(daddr)
	return KeyTuple{
		SrcIPHigh: saddrh,
		SrcIPLow:  saddrl,
		SrcPort:   sport,
		DstIPHigh: daddrh,
		DstIPLow:  daddrl,
		DstPort:   dport,
	}
}

// RequestStat stores stats for Kafka requests to a particular key
type RequestStat struct {
	Count int
}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStat) CombineWith(newStats *RequestStat) {
	r.Count += newStats.Count
}
