// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"fmt"
	"strconv"
	"strings"
)

func getInodeNumFromLink(link string) (uint64, error) {
	start := strings.LastIndexByte(link, '[')
	end := strings.LastIndexByte(link, ']')
	if start == -1 || end == -1 || start >= end-1 {
		return 0, fmt.Errorf("invalid link: %s", link)
	}
	inodeStr := link[start+1 : end]
	ino, err := strconv.ParseUint(inodeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid link: %s", link)
	}
	return ino, nil
}
