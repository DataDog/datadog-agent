// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package run

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/mallochook"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupMallochookReporter() {
	log.Debug("setting up mallochook reporter")
	inuse := telemetry.NewSimpleGauge("mallochook", "inuse",
		"Number of bytes currently allocated via malloc")
	alloc := telemetry.NewSimpleCounter("mallochook", "alloc",
		"Number of bytes allocated via malloc since the program start")

	var prevAlloc uint

	go func() {
		t := time.NewTicker(1 * time.Second)

		for range t.C {
			s := mallochook.GetStats()
			inuse.Set(float64(s.Inuse))
			alloc.Add(float64(s.Alloc - prevAlloc))
			prevAlloc = s.Alloc
		}
	}()
}
