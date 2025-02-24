// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
)

func (r *RequestStat) initSketch() error {
	latencies := protocols.SketchesPool.Get()
	if latencies == nil {
		return errors.New("error recording redis transaction latency: could not create new ddsketch")
	}
	r.Latencies = latencies
	return nil
}

func (r *RequestStat) close() {
	if r.Latencies != nil {
		r.Latencies.Clear()
		protocols.SketchesPool.Put(r.Latencies)
	}
}
