// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && cgo

package xc

/*
#include <unistd.h>
#include <sys/types.h>
#include <stdlib.h>
*/
import "C"

// GetSystemFreq grabs the system clock frequency
func GetSystemFreq() (int64, error) {
	var scClkTck C.long

	scClkTck = C.sysconf(C._SC_CLK_TCK)
	return int64(scClkTck), nil
}
