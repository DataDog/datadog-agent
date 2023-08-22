// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getPathBufferSize(c *config.Config) int {
	return int(HTTPBufferSize)
}

func getCurrentNanoSeconds() int64 {
	now, err := ebpf.NowNanoseconds()
	if err != nil {
		log.Warnf("couldn't get monotonic clock, using realtime clock instead: %s", err)
		now = time.Now().UnixNano()
	}
	return now
}
