// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"errors"
	"fmt"
)

var (
	// ErrNotEnoughData is returned when the buffer is too small to unmarshal the event
	ErrNotEnoughData = errors.New("not enough data")

	// ErrNotEnoughSpace is returned when the provided buffer is too small to marshal the event
	ErrNotEnoughSpace = errors.New("not enough space")

	// ErrStringArrayOverflow returned when there is a string array overflow
	ErrStringArrayOverflow = errors.New("string array overflow")

	// ErrNonPrintable returned when a string contains non printable char
	ErrNonPrintable = errors.New("non printable")

	// ErrIncorrectDataSize is returned when the data read size doesn't correspond to the expected one
	ErrIncorrectDataSize = errors.New("incorrect data size")
)

// ErrInvalidKeyPath is returned when inode or mountid are not valid
type ErrInvalidKeyPath struct {
	Inode   uint64
	MountID uint32
}

func (e *ErrInvalidKeyPath) Error() string {
	return fmt.Sprintf("invalid inode/mountID couple: %d/%d", e.Inode, e.MountID)
}

// ErrProcessMissingParentNode used when the lineage is incorrect in term of pid/ppid
type ErrProcessMissingParentNode struct {
	PID         uint32
	PPID        uint32
	ContainerID string
}

func (e *ErrProcessMissingParentNode) Error() string {
	return fmt.Sprintf("parent node missing: PID(%d), PPID(%d), ID(%s)", e.PID, e.PPID, e.ContainerID)
}

// ErrProcessWrongParentNode used when the lineage is correct in term of pid/ppid but an exec parent is missing
type ErrProcessWrongParentNode struct {
	PID         uint32
	PPID        uint32
	ContainerID string
}

func (e *ErrProcessWrongParentNode) Error() string {
	return fmt.Sprintf("wrong parent node: PID(%d), PPID(%d), ID(%s)", e.PID, e.PPID, e.ContainerID)
}

// ErrProcessIncompleteLineage used when the lineage is incorrect in term of pid/ppid
type ErrProcessIncompleteLineage struct {
	PID         uint32
	PPID        uint32
	ContainerID string
}

func (e *ErrProcessIncompleteLineage) Error() string {
	return fmt.Sprintf("parent node missing: PID(%d), PPID(%d), ID(%s)", e.PID, e.PPID, e.ContainerID)
}

// ErrNoProcessContext defines an error for event without process context
var ErrNoProcessContext = errors.New("process context not resolved")

// ErrProcessArgumentsMissing defines an error for event without process arguments
var ErrProcessArgumentsMissing = errors.New("process arguments not resolved")

// ErrProcessEnvVarsMissing defines an error for event without process environment variables
var ErrProcessEnvVarsMissing = errors.New("process environment variables not resolved")

// ErrProcessArgsEnvsResolution defines an error for process args/envs resolution failure
type ErrProcessArgsEnvsResolution struct {
	Err error
}

// Unwrap implements the error interface
func (e *ErrProcessArgsEnvsResolution) Unwrap() error {
	return e.Err
}

// Error implements the error interface
func (e *ErrProcessArgsEnvsResolution) Error() string {
	return fmt.Sprintf("process args/envs resolution failed: %v", e.Err)
}

// ErrProcessBrokenLineage returned when a process lineage is broken
type ErrProcessBrokenLineage struct {
	Err error
}

// Unwrap implements the error interface
func (e *ErrProcessBrokenLineage) Unwrap() error {
	return e.Err
}

// Error implements the error interface
func (e *ErrProcessBrokenLineage) Error() string {
	return fmt.Sprintf("broken process lineage: %v", e.Err)
}
