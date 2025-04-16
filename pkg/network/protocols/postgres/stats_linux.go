// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
)

// This file contains the structs used to store and combine the stats for the Postgres protocol.
// The file does not have any build tag, so it can be used in any build as it is used by the tracer package.

func (r *RequestStat) initSketch() error {
	latencies := protocols.SketchesPool.Get()
	if latencies == nil {
		return errors.New("error recording postgres transaction latency: could not create new ddsketch")
	}
	r.Latencies = latencies
	return nil
}

// Close cleans up the RequestStat
func (r *RequestStat) Close() {
	if r.Latencies != nil {
		r.Latencies.Clear()
		protocols.SketchesPool.Put(r.Latencies)
	}
}
