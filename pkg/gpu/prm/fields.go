// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

// Package prm defines GPU Performance Resource Metrics query metadata.
package prm

const (
	PPCNTGroupPLR = 0x22
)

// PLRCounterFields defines the counters returned by PPCNT group 0x22.
var PLRCounterFields = []string{
	"nvlink.plr.rx.codes",
	"nvlink.plr.rx.code_err",
	"nvlink.plr.rx.uncorrectable_code",
	"nvlink.plr.tx.codes",
	"nvlink.plr.tx.retry_codes",
	"nvlink.plr.tx.retry_events",
	"nvlink.plr.tx.sync_events",
	"nvlink.plr.codes_loss",
	"nvlink.plr.tx.retry_events_within_t_sec_max",
}
