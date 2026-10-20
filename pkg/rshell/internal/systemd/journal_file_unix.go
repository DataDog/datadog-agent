// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package systemd

import (
	"fmt"
	"math"
	"os"
	"syscall"
)

const journalBlockSize = 512

type journalFileStat struct {
	dev       uint64
	ino       uint64
	nlink     uint64
	blocks    uint64
	allocated uint64
}

func journalStat(info os.FileInfo) (journalFileStat, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Blocks < 0 {
		return journalFileStat{}, fmt.Errorf("allocation metadata is unavailable")
	}
	blocks := uint64(stat.Blocks)
	if blocks > math.MaxUint64/journalBlockSize {
		return journalFileStat{}, fmt.Errorf("allocation metadata overflows")
	}
	return journalFileStat{
		dev:       uint64(stat.Dev),
		ino:       uint64(stat.Ino),
		nlink:     uint64(stat.Nlink),
		blocks:    blocks,
		allocated: blocks * journalBlockSize,
	}, nil
}
