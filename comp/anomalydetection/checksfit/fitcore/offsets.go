// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the offsets were extracted from mapping.go so the framing
// constants stay available on every platform.

package fitcore

// Fixed offsets inside the shared mapping. The metadata region, the two
// padded index words, and the ring are agreed by the shared contract; no
// language-specific struct layout is ever overlaid on the mapping.
const (
	writeOffset = 128 // producer-owned atomic write index
	readOffset  = 256 // consumer-owned atomic read index
	ringOffset  = 384 // start of the C-byte ring
)
