// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin
// +build linux darwin

package platform

import (
	"golang.org/x/sys/unix"
)

// GetArchInfo returns basic host architecture information
func GetArchInfo() (map[string]string, error) {
	archInfo := map[string]string{}

	var uname unix.Utsname
	err := unix.Uname(&uname)
	if err != nil {
		return nil, err
	}

	updateArchInfo(archInfo, &uname)

	return archInfo, nil
}
