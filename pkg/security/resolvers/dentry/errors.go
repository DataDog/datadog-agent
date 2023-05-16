// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dentry holds dentry related files
package dentry

// ErrERPCRequestNotProcessed is used to notify that the eRPC request was not processed
type ErrERPCRequestNotProcessed struct{}

func (err ErrERPCRequestNotProcessed) Error() string {
	return "erpc_not_processed"
}

var errERPCRequestNotProcessed ErrERPCRequestNotProcessed

// // ErrTruncatedParentsERPC is used to notify that some parents of the path are missing
// type ErrTruncatedParentsERPC struct{}

// func (err ErrTruncatedParentsERPC) Error() string {
// 	return "truncated_parents_erpc"
// }

// var errTruncatedParentsERPC ErrTruncatedParentsERPC

// // ErrTruncatedParents is used to notify that some parents of the path are missing
// type ErrTruncatedParents struct{}

// func (err ErrTruncatedParents) Error() string {
// 	return "truncated_parents"
// }

// var errTruncatedParents ErrTruncatedParents

// // ErrERPCResolution is used to notify that the eRPC resolution failed
// type ErrERPCResolution struct{}

// func (err ErrERPCResolution) Error() string {
// 	return "erpc_resolution"
// }

// var errERPCResolution ErrERPCResolution

// // ErrKernelMapResolution is used to notify that the Kernel maps resolution failed
// type ErrKernelMapResolution struct{}

// func (err ErrKernelMapResolution) Error() string {
// 	return "map_resolution"
// }

// var errKernelMapResolution ErrKernelMapResolution

// // ErrDentryKeyNotFound is used to notify that the request key is missing from the kernel maps
// type ErrDentryKeyNotFound struct {
// 	DentryKey model.DentryKey
// }

// func (err ErrDentryKeyNotFound) Error() string {
// 	return fmt.Sprintf("dentry path key not found %d/%d", err.DentryKey.MountID, err.DentryKey.Inode)
// }
