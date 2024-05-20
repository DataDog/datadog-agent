// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package syscallctx holds syscall context related files
package syscallctx

import (
	"fmt"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	strMaxSize = 128 // see kernel definition
)

// SyscallCtx maps the kernel structure
type SyscallCtx struct {
	StrArg1 [strMaxSize]byte
	StrArg2 [strMaxSize]byte
	IntArg1 int64
	IntArg2 int64
}

// Resolver resolves syscall context
type Resolver struct {
	ctxMap *lib.Map
}

// Resolve resolves the syscall context
func (sr *Resolver) Resolve(ctxID uint32) (string, string, int64, int64, error) {
	var ctx SyscallCtx
	if err := sr.ctxMap.Lookup(ctxID, &ctx); err != nil {
		return "", "", 0, 0, fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
	}

	str1Arg, err := model.UnmarshalString(ctx.StrArg1[:], strMaxSize)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
	}

	str2Arg, err := model.UnmarshalString(ctx.StrArg2[:], strMaxSize)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
	}

	return str1Arg, str2Arg, ctx.IntArg1, ctx.IntArg2, nil
}

// Start the syscall context resolver
func (sr *Resolver) Start(manager *manager.Manager) error {
	pathnames, err := managerhelper.Map(manager, "syscall_ctx")
	if err != nil {
		return err
	}
	sr.ctxMap = pathnames

	return nil
}

// Close the resolver
func (sr *Resolver) Close() error {
	return nil
}

// NewResolver returns a new syscall context
func NewResolver() *Resolver {
	return &Resolver{}
}
