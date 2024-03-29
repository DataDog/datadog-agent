// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UserHZToNano holds the divisor to convert HZ to Nanoseconds
// It's safe to consider it being 100Hz in userspace mode
const (
	UserHZToNano uint64 = uint64(time.Second) / 100
)

func randToken(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func inodeForPath(path string) uint64 {
	stat, err := os.Stat(path)
	if err != nil {
		log.Debugf("unable to retrieve the inode for path %s: %v", path, err)
		return unknownInode
	}
	inode := stat.Sys().(*syscall.Stat_t).Ino
	if inode > 2 {
		return inode
	}
	return unknownInode
}
