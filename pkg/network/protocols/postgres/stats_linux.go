// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"github.com/DataDog/sketches-go/ddsketch"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file contains the structs used to store and combine the stats for the Postgres protocol.
// The file does not have any build tag, so it can be used in any build as it is used by the tracer package.

// relativeAccuracy defines the acceptable error in quantile values calculated by DDSketch.
// For example, if the actual value at p50 is 100, with a relative accuracy of 0.01 the value calculated
// will be between 99 and 101
const relativeAccuracy = 0.01

func (r *RequestStat) initSketch() (err error) {
	r.Latencies, err = ddsketch.NewDefaultDDSketch(relativeAccuracy)
	if err != nil {
		log.Debugf("error recording postgres transaction latency: could not create new ddsketch: %v", err)
	}
	return
}
