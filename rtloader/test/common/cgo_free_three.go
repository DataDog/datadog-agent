// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build three

package testcommon

/*
#cgo CFLAGS: -I../../common
#cgo !windows LDFLAGS: -L../../three/ -ldatadog-agent-three
#cgo windows LDFLAGS: -L../../three/ -ldatadog-agent-three.dll
#include "cgo_free.h"

extern void cgo_free(void *ptr);

void c_callCgoFree(void *ptr) {
	cgo_free(ptr);
}
*/
import "C"
