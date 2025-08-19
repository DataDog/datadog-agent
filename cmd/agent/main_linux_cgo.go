// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package main

/*
#include <errno.h>
#include <sys/prctl.h>
#include <stdlib.h>

int prctl_err = 0;

int set_process_name () __attribute__((constructor));

int set_process_name()
{
	const char *name = getenv("DD_BUNDLED_AGENT");
	if (name != NULL) {
		int ret = prctl(PR_SET_NAME, name, 0, 0);
		if (!ret) {
			prctl_err = errno;
		}
		return ret;
	}
	return 0;
}
*/
import (
	"C"
)
import "syscall"

func setProcessName(_ string) error {
	if C.prctl_err == 0 {
		return nil
	}
	return syscall.Errno(C.prctl_err)
}
