// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pressure implements the PSI (Pressure Stall Information) check.
// It reads host-level pressure data from /proc/pressure/{cpu,memory,io}
// and emits cumulative stall time metrics.
package pressure
