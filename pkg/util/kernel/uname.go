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
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return "", fmt.Errorf("uname: %w", err)
	}
	return unix.ByteSliceToString(uname.Release[:]), nil
})
