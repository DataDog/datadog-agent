// Copyright (c) 2017, Andrey Nering <andrey.nering@gmail.com>
// See LICENSE for licensing information

//go:build !unix

package interp

// waitStatus is a no-op on plan9 and windows.
type waitStatus struct{}

func (waitStatus) Signaled() bool { return false }
func (waitStatus) Signal() int    { return 0 }
