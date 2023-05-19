// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package telemetry

/*
#include "../../ebpf/c/telemetry_types.h"
*/
import "C"

type MapErrTelemetry C.map_err_telemetry_t
type HelperErrTelemetry C.helper_err_telemetry_t
