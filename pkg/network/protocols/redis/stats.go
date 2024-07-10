// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redis

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// Key is an identifier for a group of Redis transactions
type Key struct {
	types.ConnectionKey
}

// RequestStat represents a group of Redis transactions.
type RequestStat struct{}

func (r *RequestStat) CombineWith(*RequestStat) {}
