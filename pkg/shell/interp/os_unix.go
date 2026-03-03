// Copyright (c) 2017, Andrey Nering <andrey.nering@gmail.com>
// See LICENSE for licensing information

//go:build unix

package interp

import "syscall"

type waitStatus = syscall.WaitStatus
