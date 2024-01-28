// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

// Package tests holds tests related files
package tests

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	testEnvironment  string
	logLevelStr      string
	logPatterns      stringSlice
	logTags          stringSlice
	logStatusMetrics bool
	withProfile      bool
	trace            bool
	ebpfLessEnabled  bool
)

func SkipIfNotAvailable(t *testing.T) {
	if ebpfLessEnabled {
		available := []string{
			"~TestProcess",
			"~TestOpen",
			"~TestUnlink",
			"~KillAction",
			"~TestRmdir",
			"~TestRename",
			"~TestMkdir",
			"~TestUtimes",
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

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	if trace {
		args := slices.DeleteFunc(os.Args, func(arg string) bool {
			return arg == "-trace"
		})
		args = append(args, "-ebpfless")

		envs := os.Environ()

		err := ptracer.StartCWSPtracer(args, envs, constants.DefaultEBPFLessProbeAddr, ptracer.Creds{}, false /* verbose */, true /* async */, false /* disableStats */)
		if err != nil {
			fmt.Printf("unable to trace [%v]: %s", args, err)
			os.Exit(-1)
		}
		return
	}

	retCode := m.Run()
	if testMod != nil {
		testMod.cleanup()
	}

	if commonCfgDir != "" {
		_ = os.RemoveAll(commonCfgDir)
	}
	os.Exit(retCode)
}

func init() {
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
	flag.StringVar(&logLevelStr, "loglevel", seelog.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
	flag.BoolVar(&logStatusMetrics, "status-metrics", false, "display status metrics")
	flag.BoolVar(&withProfile, "with-profile", false, "enable profile per test")
	flag.BoolVar(&trace, "trace", false, "wrap the test suite with the ptracer")
	flag.BoolVar(&ebpfLessEnabled, "ebpfless", false, "enabled the ebpfless mode")

	rand.Seed(time.Now().UnixNano())

	testSuitePid = utils.Getpid()
}
