// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !windows

// Package main implements 'updater'.
package main

import (
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

func moveToDDUpdater() error {
	usr, err := user.Lookup("dd-updater")
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return err
	}
	return syscall.Setuid(uid)
}

func rootToDDUpdater() {
	userID := syscall.Getuid()
	if userID != 0 {
		return
	}
	fmt.Println("Program run as root, downgrading to dd-updater user.")

	if err := moveToDDUpdater(); err != nil {
		fmt.Printf("Failed to downgrade to dd-updater user, running as root: %v\n", err)
	}
}
