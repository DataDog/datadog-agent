// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func isProcessContainerized(pid int, rootPIDNamespace string) bool {
	processPIDNamespace, err := getPIDNamespace(pid)
	if err != nil {
		return false
	}

	return processPIDNamespace != rootPIDNamespace
}

func getPIDNamespace(pid int) (string, error) {
	pidNamespacePath := kernel.HostProc(strconv.Itoa(pid), "ns", "pid")

	pidNamespace, err := os.Readlink(pidNamespacePath)
	if err != nil {
		return "", fmt.Errorf("could not read process %d PID namespace: %w", pid, err)
	}

	// Readlink read a string with the following format: pid:[<id>]
	// We only care about the <id> portion, which we extract here.

	// Length of "pid:["
	const nsPrefixLen = 5

	closingBracketIndex := len(pidNamespace) - 1

	if closingBracketIndex < nsPrefixLen {
		return "", errors.New("read invalid PID namespace")
	}

	return pidNamespace[nsPrefixLen:closingBracketIndex], nil
}
