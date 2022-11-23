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
)

type testLogger struct {
	t *testing.T
}

func (l *testLogger) Info(params ...interface{}) {
	l.t.Log(params...)
}

func (l *testLogger) Infof(format string, params ...interface{}) {
	l.t.Logf(format, params...)
}

func (l *testLogger) Debug(params ...interface{}) {
	l.t.Log(params...)
}

func (l *testLogger) Debugf(format string, params ...interface{}) {
	l.t.Logf(format, params...)
}

func (l *testLogger) Error(params ...interface{}) {
	l.t.Error(params...)
}

func (l *testLogger) Errorf(format string, params ...interface{}) {
	l.t.Errorf(format, params...)
}

func newVM(t *testing.T) *baloum.VM {
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
		t.Fatal(err)
	}

	fncs := baloum.Fncs{
		TracePrintk: func(vm *baloum.VM, format string, args ...interface{}) error {
			t.Logf(format, args...)
			return nil
		},
	}

	return baloum.NewVM(spec, baloum.Opts{Fncs: fncs, Logger: &testLogger{t: t}})
}

func TestDiscarderEventMask(t *testing.T) {
	var ctx baloum.Context

	code, err := newVM(t).RunProgram(ctx, "test/discarders_event_mask")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}

func TestDiscarderRetention(t *testing.T) {
	var ctx baloum.Context

	code, err := newVM(t).RunProgram(ctx, "test/discarders_retention")
	if err != nil || code != 0 {
		t.Errorf("unexpected error: %v, %d", err, code)
	}
}
