// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package postgres

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

var (
	testInterner = intern.NewStringInterner()
)

// NewKey creates a new postgres key
func NewKey(saddr, daddr util.Address, sport, dport uint16, operation Operation, parameters string) Key {
	return Key{
		ConnectionKey: types.NewConnectionKey(saddr, daddr, sport, dport),
		Operation:     operation,
		Parameters:    testInterner.GetString(parameters),
	}
}
