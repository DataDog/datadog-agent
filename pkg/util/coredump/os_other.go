// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !linux && !darwin

package coredump

import (
	"errors"
	"os"
	"path/filepath"
)

var errNotSupported = errors.New("not supported on this platform")

var nameSpecifiers = map[byte]int{}

func readCorePattern() (string, error) { return "", errNotSupported }

func procName() string { return filepath.Base(os.Args[0]) }

func freeBytes(string) (uint64, error) { return 0, errNotSupported }

func setCoreLimit(uint64, uint64) error { return errNotSupported }

func getCoreLimit() (uint64, uint64, error) { return 0, 0, errNotSupported }
