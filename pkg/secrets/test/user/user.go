// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func getUsername() (string, error) {
	t, e := syscall.OpenCurrentProcessToken()
	if e != nil {
		return "", e
	}
	defer t.Close()
	u, e := t.GetTokenUser()
	if e != nil {
		return "", e
	}

	username, _, accType, e := u.User.Sid.LookupAccount("")
	if e != nil {
		return "", e
	}
	if accType != syscall.SidTypeUser {
		return "", fmt.Errorf("user: should be user account type, not %d", t)
	}
	return username, nil
}

func main() {
	username, err := getUsername()
	if err != nil {
		fmt.Printf("Could not retrieve current user: %s\n", err)
		os.Exit(0)
	}
	fmt.Printf("Username: %s", username)
}
