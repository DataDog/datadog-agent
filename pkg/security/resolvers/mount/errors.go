// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	"errors"
	"fmt"
)

var (
	// ErrMountUndefined is used when a mount identifier is undefined
	ErrMountUndefined = errors.New("undefined mountID")
	// ErrMountLoop is returned when there is a resolution loop
	ErrMountLoop = errors.New("mount resolution loop")
	// ErrMountPathEmpty is returned when the resolved mount path is empty
	ErrMountPathEmpty = errors.New("mount resolution return empty path")
	// ErrMountKernelID is returned when it's not a critical error
	ErrMountKernelID = errors.New("not a critical error")
)

// ErrMountNotFound is used when an unknown mount identifier is found
type ErrMountNotFound struct {
	MountID uint32
}

func (e *ErrMountNotFound) Error() string {
	return fmt.Sprintf("mount ID not found: %d", e.MountID)
}
