// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ignore

package noisyneighbor

/*
#include "../../c/runtime/noisy-neighbor-kern-user.h"
*/
import "C"

type ebpfCgroupAggStats C.cgroup_agg_stats_t
type ebpfPmuCounter C.pmu_counter_t
type ebpfPmuCgroupStats C.pmu_cgroup_stats_t
type ebpfPmuConfig C.pmu_config_t
type ebpfPmuErrorStats C.pmu_error_stats_t
