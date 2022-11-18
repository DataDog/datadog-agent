// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata
// +build linux,ebpf_bindata

package tests

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	secebpf "github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cilium/ebpf"
	"github.com/safchain/baloum/pkg/baloum"
	"go.uber.org/zap"
)

func TestDiscarderRetention(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	suggar := logger.Sugar()

	useSyscallWrapper, err := secebpf.IsSyscallWrapperRequired()
	if err != nil {
		t.Fatal(err)
	}

	loader := secebpf.NewProbeLoader(&config.Config{}, useSyscallWrapper, &statsd.NoOpClient{})
	reader, _, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(reader)
	if err != nil {
		suggar.Fatal(err)
	}

	tgid := uint64(33)

	fncs := baloum.Fncs{
		GetCurrentPidTgid: func(vm *baloum.VM) (uint64, error) {
			return tgid, nil
		},
		TracePrintk: func(vm *baloum.VM, format string, args ...interface{}) {
			suggar.Debugf(format, args...)
		},
	}

	vm := baloum.NewVM(spec, baloum.Opts{Fncs: fncs, Logger: suggar})

	var ctx baloum.Context

	code, err := vm.RunProgram(ctx, "test/discarders_retention")
	if err != nil || code != 0 {
		suggar.Fatalf("unexpected error: %v, %d", err, code)
	}
}
