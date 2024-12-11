// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package ebpf

/*
#include "./c/lock_contention.h"
*/
import "C"

type LockRange C.lock_range_t
type ContentionData C.contention_data_t
type LockType C.lock_type_t
