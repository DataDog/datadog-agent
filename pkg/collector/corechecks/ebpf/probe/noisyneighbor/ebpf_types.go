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
