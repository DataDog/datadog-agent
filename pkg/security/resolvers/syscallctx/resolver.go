// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package syscallctx holds syscall context related files
package syscallctx

import (
	"encoding/binary"
	"fmt"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	argMaxSize = 128 // see kernel definition

	// types // see kernel definition
	strArg = 1
	intArg = 2
)

// KernelSyscallCtxStruct maps the kernel structure
type KernelSyscallCtxStruct struct {
	Types uint8
	Arg1  [argMaxSize]byte
	Arg2  [argMaxSize]byte
	Arg3  [argMaxSize]byte
}

// Resolver resolves syscall context
type Resolver struct {
	ctxMap *lib.Map
}

// Resolve resolves the syscall context
func (sr *Resolver) Resolve(ctxID uint32, ctx *model.SyscallContext) error {
	var ks KernelSyscallCtxStruct
	if err := sr.ctxMap.Lookup(ctxID, &ks); err != nil {
		return fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
	}

	isStrArg := func(pos int) bool {
		return (ks.Types>>(pos*2))&strArg > 0
	}

	isIntArg := func(pos int) bool {
		return (ks.Types>>(pos*2))&intArg > 0
	}

	if isStrArg(0) {
		arg, err := model.UnmarshalString(ks.Arg1[:], argMaxSize)
		if err != nil {
			return fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
		}
		ctx.CtxStrArg1 = arg
	} else if isIntArg(0) {
		ctx.CtxIntArg1 = int64(binary.NativeEndian.Uint64(ks.Arg1[:]))
	}

	if isStrArg(1) {
		arg, err := model.UnmarshalString(ks.Arg2[:], argMaxSize)
		if err != nil {
			return fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
		}
		ctx.CtxStrArg2 = arg
	} else if isIntArg(1) {
		ctx.CtxIntArg2 = int64(binary.NativeEndian.Uint64(ks.Arg2[:]))
	}

	if isStrArg(2) {
		arg, err := model.UnmarshalString(ks.Arg3[:], argMaxSize)
		if err != nil {
			return fmt.Errorf("unable to resolve the syscall context for `%d`: %w", ctxID, err)
		}
		ctx.CtxStrArg3 = arg
	} else if isIntArg(2) {
		ctx.CtxIntArg3 = int64(binary.NativeEndian.Uint64(ks.Arg3[:]))
	}

	return nil
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
