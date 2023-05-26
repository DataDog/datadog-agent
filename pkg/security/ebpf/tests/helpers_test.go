// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata

package tests

import (
	"flag"
	"os"
	"testing"
	"time"

	secebpf "github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cilium/ebpf"
	"github.com/safchain/baloum/pkg/baloum"
)

type testLogger struct {
	t     *testing.T
	trace bool
}

func (l *testLogger) Info(params ...interface{}) {
	l.t.Log(params...)
}

func (l *testLogger) Infof(format string, params ...interface{}) {
	l.t.Logf(format, params...)
}

func (l *testLogger) Debug(params ...interface{}) {
	if l.trace {
		l.t.Log(params...)
	}
}

func (l *testLogger) Debugf(format string, params ...interface{}) {
	if l.trace {
		l.t.Logf(format, params...)
	}
}

func (l *testLogger) Error(params ...interface{}) {
	l.t.Error(params...)
}

func (l *testLogger) Errorf(format string, params ...interface{}) {
	l.t.Errorf(format, params...)
}

var trace bool

func newVM(t *testing.T) *baloum.VM {
	useSyscallWrapper, err := secebpf.IsSyscallWrapperRequired()
	if err != nil {
		t.Fatal(err)
	}

	loader := secebpf.NewProbeLoader(&config.Config{}, useSyscallWrapper, false, &statsd.NoOpClient{})
	reader, _, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(reader)
	if err != nil {
		t.Fatal(err)
	}

	var now time.Time

	fncs := baloum.Fncs{
		TracePrintk: func(vm *baloum.VM, format string, args ...interface{}) error {
			t.Logf(format, args...)
			return nil
		},
		// fake the time duration to speed up the tests
		KtimeGetNS: func(vm *baloum.VM) (uint64, error) {
			return uint64(now.UnixNano()), nil
		},
		Sleep: func(vm *baloum.VM, duration time.Duration) error {
			now = now.Add(duration)
			return nil
		},
	}

	return baloum.NewVM(spec, baloum.Opts{Fncs: fncs, Logger: &testLogger{t: t, trace: trace}})
}

func TestMain(m *testing.M) {
	flag.BoolVar(&trace, "trace", false, "enable eBPF VM instruction tracing")
	flag.Parse()
	os.Exit(m.Run())
}
