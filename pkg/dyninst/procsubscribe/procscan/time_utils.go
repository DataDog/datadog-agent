// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package procscan

import (
	"time"
)

// CLK_TCK is a constant on Linux for all architectures except alpha and ia64.
// See e.g.
// https://git.musl-libc.org/cgit/musl/tree/src/conf/sysconf.c#n30
// https://github.com/containerd/cgroups/pull/12
// https://lore.kernel.org/lkml/agtlq6$iht$1@penguin.transmeta.com/
//
// See https://github.com/tklauser/go-sysconf/blob/e2b5de3c/sysconf_linux.go#L19-L24
const clkTck = 100

// ticks is a type for representing time in ticks.
type ticks uint64

// durationToTicks converts a duration to USER_HZ ticks, truncating.
func durationToTicks(d time.Duration) ticks {
	return ticks((d.Nanoseconds() * clkTck) / time.Second.Nanoseconds())
}
