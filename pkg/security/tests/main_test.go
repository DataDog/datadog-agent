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
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
	"golang.org/x/exp/slices"
)

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()

	if trace {
		args := slices.DeleteFunc(os.Args, func(arg string) bool {
			return arg == "-trace"
		})

		envs := os.Environ()
		envs = append(envs, "EBPFLESS=true")

		err := ptracer.StartCWSPtracer(args, envs, setup.DefaultEBPFLessProbeAddr, ptracer.Creds{}, false /* verbose */, true /* async */, false /* disableStats */)
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
