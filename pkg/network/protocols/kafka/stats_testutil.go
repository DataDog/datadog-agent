// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package kafka

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

var (
	testInterner = intern.NewStringInterner()
)

// NewKey generates a new Key
func NewKey(saddr, daddr util.Address, sport, dport uint16, topicName string, requestAPIKey, requestAPIVersion uint16) Key {
	return Key{
		ConnectionKey:  types.NewConnectionKey(saddr, daddr, sport, dport),
		TopicName:      testInterner.GetString(topicName),
		RequestAPIKey:  requestAPIKey,
		RequestVersion: requestAPIVersion,
	}
}
