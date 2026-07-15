// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package eventbuf buffers probe-event fragments on the userspace side of the
// dyninst dataplane.
//
// It handles two related concerns:
//
//  1. Fragment reassembly: a single logical event may be split across multiple
//     ringbuf submissions (continuation fragments). The buffer accumulates
//     fragments under an invocation-scoped key and surfaces the complete
//     message list once the final fragment arrives.
//
//  2. Entry/return pairing: an entry event that precedes a return probe is
//     stored until the matching return event arrives, at which point they are
//     paired and passed on together.
//
// The buffer is single-goroutine; callers serialize access.
package eventbuf
