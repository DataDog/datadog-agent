// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test

package redis

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// NewKey creates a new redis key
func NewKey(saddr, daddr util.Address, sport, dport uint16, command CommandType, keyName string, truncated bool) Key {
	return Key{
		ConnectionKey: types.NewConnectionKey(saddr, daddr, sport, dport),
		Command:       command,
		KeyName:       Interner.GetString(keyName),
		Truncated:     truncated,
	}
}
