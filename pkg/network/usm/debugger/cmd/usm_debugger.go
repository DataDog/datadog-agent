// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build usm_debugger

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

func main() {
	err := pkglogsetup.SetupLogger(
		"usm-debugger",
		"debug",
		"",
		pkglogsetup.GetSyslogURI(pkgconfigsetup.Datadog()),
		false,
		true,
		false,
		pkgconfigsetup.Datadog(),
	)
	checkError(err)

	cleanupFn := setupBytecode()
	defer cleanupFn()

	monitor, err := usm.NewMonitor(getConfiguration(), nil, nil)
	checkError(err)

	err = monitor.Start()
	checkError(err)

	go func() {
		t := time.NewTicker(10 * time.Second)
		for range t.C {
			_, cleaners = monitor.GetProtocolStats()
			cleaners()
		}
	}()

	defer monitor.Stop()
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(-1)
	}
}

func getConfiguration() *networkconfig.Config {
	// done for the purposes of initializing the configuration values
	_, err := config.New("", "")
	checkError(err)

	c := networkconfig.New()

	// run debug version of the eBPF program
	c.BPFDebug = true
	c.EnableUSMEventStream = false

	// don't buffer data in userspace
	// this is to ensure that we won't inadvertently trigger an OOM kill
	// by enabling the debugger inside a system-probe container.
	c.MaxHTTPStatsBuffered = 0
	c.MaxKafkaStatsBuffered = 0

	// make sure we use the CO-RE compilation artifact embedded
	// in this build (see `ebpf_bytecode.go`)
	c.EnableCORE = true
	c.EnableRuntimeCompiler = false

	return c
}
