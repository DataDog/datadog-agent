// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpfless

// Package probe holds probe related files
package probe

import (
	"context"
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/safchain/rstrace/pkg/rstrace"
	"google.golang.org/grpc"
)

type PlatformProbe struct {
	// internals
	rstrace.UnimplementedSyscallStreamServer
}

func (p *Probe) SendSyscall(ctx context.Context, syscall *rstrace.Syscall) (*rstrace.Response, error) {
	fmt.Printf(">>: %+v\n", syscall)
	return &rstrace.Response{}, nil
}

func (p *Probe) Setup() error {
	return nil
}

func (p *Probe) Init() error {
	return nil
}

func (p *Probe) Snapshot() error {
	return nil
}

func (p *Probe) Stop() {}

func (p *Probe) Close() error {
	return nil
}

func (p *Probe) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 7878))
	if err != nil {
		return err
	}
	var opts []grpc.ServerOption

	grpcServer := grpc.NewServer(opts...)
	rstrace.RegisterSyscallStreamServer(grpcServer, p)

	grpcServer.Serve(lis)
	return nil
}

func NewProbe(config *config.Config) (*Probe, error) {
	return &Probe{
		Config: config,
	}, nil
}
