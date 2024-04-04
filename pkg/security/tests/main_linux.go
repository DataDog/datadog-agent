// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && (functionaltests || stresstests)

// Package tests holds tests related files
package tests

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	fakePasswdPath = "/tmp/fake_passwd"
	fakeGroupPath  = "/tmp/fake_group"
)

func SkipIfNotEBPFLess(t *testing.T) {
	if !ebpfLessEnabled {
		t.Skip("only supports ebpfless")
	}
}

func SkipIfNotAvailable(t *testing.T) {
	if ebpfLessEnabled {
		if testEnvironment == DockerEnvironment {
			t.Skip("skipping ebpfless test in docker")
		}

		available := []string{
			"~TestProcess",
			"~TestOpen",
			"~TestUnlink",
			"~TestActionKill",
			"~TestRmdir",
			"~TestRename",
			"~TestMkdir",
			"~TestUtimes",
			"~TestHardLink",
			"~TestLink",
			"~TestChmod",
			"~TestChown",
			"~TestLoadModule",
			"~TestUnloadModule",
			"~TestOsOrigin",
		}

		exclude := []string{
			"TestMkdir/io_uring",
			"TestOpenDiscarded",
			"TestOpenDiscarded/pipefs",
			"TestOpen/truncate",
			"TestOpen/io_uring",
			"TestProcessContext/inode",
			"TestProcessContext/pid1",
			"~TestProcessBusybox",
			"TestRename/io_uring",
			"TestRenameReuseInode",
			"TestUnlink/io_uring",
			"TestRmdir/unlinkat-io_uring",
			"TestHardLinkExecsWithERPC",
			"TestHardLinkExecsWithMaps",
			"TestLink/io_uring",
			"TestLoadModule/load_module_with_truncated_params",
			"~TestChown32",
		}

		match := func(list []string) bool {
			var match bool

			for _, value := range list {
				if value[0] == '~' {
					if strings.HasPrefix(t.Name(), value[1:]) {
						match = true
						break
					}
				} else if value == t.Name() {
					match = true
					break
				}
			}

			return match
		}

		if !match(available) || match(exclude) {
			t.Skip("test not available for ebpfless")
		}
	}
}

func preTestsHook() {
	if trace {
		args := slices.DeleteFunc(os.Args, func(arg string) bool {
			return arg == "-trace"
		})
		args = append(args, "-ebpfless")

		os.Setenv(ptracer.EnvPasswdPathOverride, fakePasswdPath)
		os.Setenv(ptracer.EnvGroupPathOverride, fakeGroupPath)

		envs := os.Environ()

		opts := ptracer.Opts{
			Async: true,
		}

		err := ptracer.StartCWSPtracer(args, envs, constants.DefaultEBPFLessProbeAddr, opts)
		if err != nil {
			fmt.Printf("unable to trace [%v]: %s", args, err)
			os.Exit(-1)
		}
		os.Exit(0)
	}
}

func postTestsHook() {
	if testMod != nil {
		testMod.cleanup()
	}
}

var (
	testEnvironment  string
	logStatusMetrics bool
	withProfile      bool
	trace            bool
	disableTracePipe bool
)

var testSuitePid uint32

func init() {
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
	flag.BoolVar(&logStatusMetrics, "status-metrics", false, "display status metrics")
	flag.BoolVar(&withProfile, "with-profile", false, "enable profile per test")
	flag.BoolVar(&trace, "trace", false, "wrap the test suite with the ptracer")
	flag.BoolVar(&disableTracePipe, "no-trace-pipe", false, "disable the trace pipe log")

	testSuitePid = utils.Getpid()
}
