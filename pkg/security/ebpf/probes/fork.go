// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

// SchedProcessForkTracepointName is the function name of the sched_process_fork tracepoint
const SchedProcessForkTracepointName = "sched_process_fork"
