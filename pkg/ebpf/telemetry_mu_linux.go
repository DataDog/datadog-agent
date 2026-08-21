// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import "sync"

// TelemetryMu synchronizes Manager.Start (which writes PerfMap/RingBuffer
// telemetry fields) with perfUsageCollector.Collect (which reads them).
// It lives here (in pkg/ebpf) rather than in pkg/ebpf/telemetry to avoid
// a circular import: telemetry imports ebpf, so ebpf cannot import telemetry.
var TelemetryMu sync.Mutex
