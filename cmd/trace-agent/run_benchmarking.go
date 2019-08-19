// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build benchmarking

package main

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
)

func init() {
	runCmd.Flags().StringVar(&metrics.StatsOut, "stats-out", fmt.Sprintf("metrics-%s.stats", time.Now().Format("02-01-2006-15:04:05")), "file to write metrics to")
}
