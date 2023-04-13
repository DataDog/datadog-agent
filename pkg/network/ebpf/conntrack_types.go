// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package ebpf

/*
#include "./c/conntrack-types.h"
*/
import "C"

type ConntrackTuple C.conntrack_tuple_t

type ConntrackTelemetry C.conntrack_telemetry_t
