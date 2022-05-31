// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux
// +build linux

package erpc

const (
	// DiscardInodeOp discards an inode
	DiscardInodeOp = iota + 1
	// DiscardPidOp discards a pid
	DiscardPidOp
	// ResolveSegmentOp resolves the requested segment
	ResolveSegmentOp
	// ResolvePathOp resolves the requested path
	ResolvePathOp
	// ResolveParentOp resolves the parent of the provide path key
	ResolveParentOp
	// RegisterSpanTLSOP is used for span TLS registration
	RegisterSpanTLSOP //nolint:deadcode,unused
	// ExpireInodeDiscarderOp is used to expire an inode discarder
	ExpireInodeDiscarderOp
)
