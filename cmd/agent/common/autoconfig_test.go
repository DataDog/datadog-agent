// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/logs"
)

func TestBlockUntilAutoConfigRanOnce(t *testing.T) {
	SetupAutoConfig("/tmp")
	start := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		AC.LoadAndRun()
	}()
	logs.BlockUntilAutoConfigRanOnce(func() *autodiscovery.AutoConfig { return AC }, 2*time.Second)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("should not have timeout")
	}
}

func TestBlockUntilAutoConfigRanOnceTimeout(t *testing.T) {
	SetupAutoConfig("/tmp")
	start := time.Now()
	logs.BlockUntilAutoConfigRanOnce(func() *autodiscovery.AutoConfig { return AC }, 3*time.Second)
	if time.Since(start) < 2500*time.Millisecond {
		t.Fatalf("should have timeout")
	}
}
