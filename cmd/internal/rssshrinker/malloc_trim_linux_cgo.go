// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && cgo

package rssshrinker

/*
#include <malloc.h>

static int dd_malloc_trim(unsigned int pad) {
#if defined(__GLIBC__)
	return malloc_trim(pad);
#else
	return 0;
#endif
}
*/
import "C"

func mallocTrim() {
	if !isEnvEnabled(MallocTrimEnvVar) {
		return
	}

	C.dd_malloc_trim(0)
}
