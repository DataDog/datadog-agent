// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ignore

package oomkill

/*
#include "../../c/runtime/oom-kill-kern-user.h"
*/
import "C"

type oomStats C.struct_oom_stats

// oomStatsV2 is the version marker of struct oom_stats, see oom_stats_layout_linux.go
type oomStatsV2 C.oom_stats_v2

type cgroupMemoryStats C.struct_cgroup_memory_stats

type statsKey C.struct_stats_key
