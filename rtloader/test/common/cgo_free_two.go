// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build two

package testcommon

/*
#cgo !windows LDFLAGS: -L../../two/ -ldatadog-agent-two
#cgo windows LDFLAGS: -L../../two/ -ldatadog-agent-two.dll
#include "cgo_free.h"

void c_callCgoFree(void *ptr) {
	cgo_free(ptr);
}
*/
import "C"
