// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#ifndef SYS_execve
#if __x86_64__
#define SYS_execve 59
#elif __aarch64__
#define SYS_execve 221
#else
#error unknown architecture
#endif
#endif

// This library is meant to be used as a preload library to break the echo command
// and is not meant to be used anywhere else

int old_execve(const char *path, char *const argv[], char *const envp[]) {
	return syscall(SYS_execve, path, argv, envp);
}
int execve(const char *filename, char *const argv[], char *const envp[]) {
	if (strcmp(filename, "/bin/echo") == 0) {
	_exit(123);
	}
	return old_execve(filename, argv, envp);
}
