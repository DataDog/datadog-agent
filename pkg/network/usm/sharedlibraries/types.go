// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package sharedlibraries

/*
#include "../../ebpf/c/shared-libraries/types.h"
*/
import "C"

type libPath C.lib_path_t

const (
	libPathMaxSize = C.LIB_PATH_MAX_SIZE
)
