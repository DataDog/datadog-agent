// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package tcpqueuelength

/*
#include "../../c/runtime/tcp-queue-length-kern-user.h"
*/
import "C"

type StructStatsKey C.struct_stats_key
type StructStatsValue C.struct_stats_value
