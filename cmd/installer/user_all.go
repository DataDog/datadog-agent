// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !windows

// Package main implements 'installer'.
package main

import (
	"fmt"
	"os/user"
	"strconv"
	"syscall"
)

func moveToDDAgent() error {
	grp, err := user.LookupGroup("dd-agent")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return err
	}
	if err := syscall.Setgid(gid); err != nil {
		return err
	}
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return err
	}
	usr, err := user.Lookup("dd-agent")
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return err
	}
	return syscall.Seteuid(uid)
}

func rootToDDAgent() {
	userID := syscall.Getuid()
	if userID != 0 {
		return
	}
	fmt.Println("Program run as root, downgrading to dd-agent user and group.")

	if err := moveToDDAgent(); err != nil {
		fmt.Printf("Failed to downgrade to dd-agent user, running as root: %v\n", err)
	}
}
