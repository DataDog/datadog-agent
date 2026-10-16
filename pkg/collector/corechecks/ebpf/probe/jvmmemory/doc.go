// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package jvmmemory is the system-probe side of the jvm_memory check.
//
// It attaches eBPF programs to the HotSpot USDT probes gc__begin, gc__end and
// mem__pool__gc__end of every libjvm.so mapped by a process of the host, and
// reports the memory pools and the stop-the-world GC pauses of each JVM.
//
// The JVMs are discovered by polling procfs. The probes are attached once
// per libjvm.so file, system-wide, and detached when the last process
// mapping it exits. The programs reading the probe arguments are cloned once
// per distinct USDT argument spec. It requires the linux and bpf build tags.
package jvmmemory
