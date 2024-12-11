// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redis

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Key is an identifier for a group of Redis transactions
type Key struct {
	types.ConnectionKey
}

// NewKey creates a new redis key
func NewKey(saddr, daddr util.Address, sport, dport uint16) Key {
	return Key{
		ConnectionKey: types.NewConnectionKey(saddr, daddr, sport, dport),
	}
}

// RequestStat represents a group of Redis transactions.
type RequestStat struct{}

// CombineWith merges the data in 2 RequestStats objects
// newStats is kept as it is, while the method receiver gets mutated
func (r *RequestStat) CombineWith(*RequestStat) {}
