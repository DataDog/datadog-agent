// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// Release is the equivalent of uname -r
var Release = funcs.Memoize(func() (string, error) {
	u, err := uname()
	if err != nil {
		return "", err
	}
	return unix.ByteSliceToString(u.Release[:]), nil
})

// Machine is the equivalent of uname -m
var Machine = funcs.Memoize(func() (string, error) {
	u, err := uname()
	if err != nil {
		return "", err
	}
	return unix.ByteSliceToString(u.Machine[:]), nil
})

// UnameVersion is the equivalent of uname -v
var UnameVersion = funcs.Memoize(func() (string, error) {
	u, err := uname()
	if err != nil {
		return "", err
	}
	return unix.ByteSliceToString(u.Version[:]), nil
})

var uname = funcs.Memoize(func() (unix.Utsname, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return unix.Utsname{}, fmt.Errorf("uname: %w", err)
	}
	return u, nil
})
