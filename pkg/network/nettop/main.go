// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NET) Fix revive linter
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	networkConfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
)

func main() {
	cfgpath := flag.String("config", "/etc/datadog-agent/datadog.yaml", "The Datadog main configuration file path")
	flag.Parse()

	if supported, err := tracer.IsTracerSupportedByOS(nil); !supported {
		fmt.Fprintf(os.Stderr, "system-probe is not supported: %s\n", err)
		os.Exit(1)
	}

	config.Datadog().SetConfigFile(*cfgpath)
	if _, err := config.LoadWithoutSecret(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	cfg := networkConfig.New()
	fmt.Printf("-- Config: %+v --\n", cfg)
	cfg.BPFDebug = true

	t, err := tracer.NewTracer(cfg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Initialization complete. Starting nettop\n")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	printConns := func(now time.Time) {
		fmt.Printf("-- %s --\n", now)
		cs, err := t.GetActiveConnections(fmt.Sprintf("%d", os.Getpid()))
		if err != nil {
			fmt.Println(err)
		}
		for _, c := range cs.Conns {
			fmt.Println(network.ConnectionSummary(&c, cs.DNS))
		}
	}

	stopChan := make(chan struct{})
	go func() {
		// Print active connections immediately, and then again every 5 seconds
		tick := time.NewTicker(5 * time.Second)
		printConns(time.Now())
		for {
			select {
			case now := <-tick.C:
				printConns(now)
			case <-stopChan:
				tick.Stop()
				return
			}
		}
	}()

	<-sig
	stopChan <- struct{}{}

	t.Stop()
}
