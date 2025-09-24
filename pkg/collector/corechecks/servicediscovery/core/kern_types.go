// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package core

/*
#include "../c/ebpf/runtime/discovery-types.h"
*/
import "C"

type NetworkStatsKey C.struct_network_stats_key
type NetworkStats C.struct_network_stats
