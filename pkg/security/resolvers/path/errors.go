// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package path holds path related files
package path

import (
	"errors"
	"fmt"
)

// ErrPathResolutionNotCritical defines a non critical error
type ErrPathResolutionNotCritical struct {
	Err error
}

// Error implements the error interface
func (e *ErrPathResolutionNotCritical) Error() string {
	return fmt.Errorf("non critical path resolution error: %w", e.Err).Error()
}

// Unwrap implements the error interface
func (e *ErrPathResolutionNotCritical) Unwrap() error {
	return e.Err
}

// ErrPathResolution defines a non critical error
type ErrPathResolution struct {
	Err error
}

// Error implements the error interface
func (e *ErrPathResolution) Error() string {
	return fmt.Errorf("path resolution error: %w", e.Err).Error()
}

// Unwrap implements the error interface
func (e *ErrPathResolution) Unwrap() error {
	return e.Err
}

// ErrTruncatedPath is used to notify that a path was truncated
type ErrTruncatedPath struct{}

func (err ErrTruncatedPath) Error() string {
	return "truncated_path"
}

type pathRingsResolutionFailureCause uint32

const (
	drUnknown pathRingsResolutionFailureCause = iota
	drInvalidInode
	drDentryDiscarded
	drDentryResolution
	drDentryBadName
	drDentryMaxTailCall
	pathRefLengthTooBig
	pathRefLengthZero
	pathRefLengthTooSmall
	pathRefReadCursorOOB
	pathRefInvalidCPU
	pathRingsReadOverflow
	invalidFrontWatermarkSize
	invalidBackWatermarkSize
	frontWatermarkValueMismatch
	backWatermarkValueMismatch
	maxPathResolutionFailureCause // must be the last one
)

var pathRingsResolutionFailureCauses = [maxPathResolutionFailureCause]string{
	"unknown",
	"invalid_inode",
	"discarded_dentry",
	"dentry_resolution_error",
	"dentry_bad_name",
	"dentry_tailcall_limit",
	"too_big",
	"zero_length",
	"too_small",
	"out_of_bounds",
	"invalid_cpu",
	"read_overflow",
	"invalid_front_watermark_size",
	"invalid_back_watermark_size",
	"front_watermark_mismatch",
	"back_watermark_mismatch",
}

func (cause pathRingsResolutionFailureCause) String() string {
	return pathRingsResolutionFailureCauses[cause]
}

var (
	errDrUnknown                   = errors.New("unknown dentry resolution error")
	errDrInvalidInode              = errors.New("dentry with invalid inode")
	errDrDentryDiscarded           = errors.New("dentry discarded")
	errDrDentryResolution          = errors.New("dentry resolution error")
	errDrDentryBadName             = errors.New("dentry bad name")
	errDrDentryMaxTailCall         = errors.New("dentry tailcall limit reached")
	errPathRefLengthTooBig         = errors.New("path ref length exceeds ring buffer size")
	errPathRefLengthZero           = errors.New("path ref length is zero")
	errPathRefLengthTooSmall       = errors.New("path ref length is too small")
	errPathRefReadCursorOOB        = errors.New("path ref read cursor is out-of-bounds")
	errPathRefInvalidCPU           = errors.New("path ref cpu is invalid")
	errPathRingsReadOverflow       = errors.New("read from path rings map overflow")
	errInvalidFrontWatermarkSize   = errors.New("front watermark read from path rings map has invalid size")
	errInvalidBackWatermarkSize    = errors.New("back watermark read from path rings map has invalid size")
	errFrontWatermarkValueMismatch = errors.New("mismatch between path ref watermark and front watermark from path rings")
	errBackWatermarkValueMismatch  = errors.New("mismatch between path ref watermark and back watermark from path rings")
)
