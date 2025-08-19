// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
)

func requestFragment(fragment []byte, buffsize int) []byte {
	b := make([]byte, buffsize)

	copy(b[:], fragment)
	return b
}

func getBufferSize() int {
	cfg := config.New()
	return int(cfg.HTTPMaxRequestFragment)
}

func makeTxnFromRequestString(s string) WinHttpTransaction {
	return WinHttpTransaction{
		RequestFragment: requestFragment([]byte(s), getBufferSize()),
	}
}

func makeTxnFromLatency(lastSeen, started uint64) WinHttpTransaction {
	return WinHttpTransaction{
		Txn: driver.HttpTransactionType{
			ResponseLastSeen: lastSeen,
			RequestStarted:   started,
		},
	}
}
