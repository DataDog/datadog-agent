// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

// Package main implements a standalone macOS Workload Protection collector proof
// of concept: it reads Endpoint Security events from /usr/bin/eslogger, evaluates
// them against SECL rules, and ships matches to the runtime-security intake.
//
// This deliberately does not go through system-probe or the security-agent. The
// point is to get a real macOS event in front of the backend without first
// porting pkg/security/probe and pkg/eventmonitor, both of which are
// linux || windows.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/darwin"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	logsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

func main() {
	var (
		hostname    = flag.String("hostname", "", "hostname to report (required)")
		site        = flag.String("site", "datad0g.com", "Datadog site")
		policiesDir = flag.String("policies", "pkg/security/darwin/policies", "policy directory")
		logLevel    = flag.String("log-level", "info", "log level")
	)
	flag.Parse()

	events := flag.Args()
	if len(events) == 0 {
		events = []string{"exec", "fork", "exit"}
	}

	// Without an explicit hostname the intake cannot attribute events, and there
	// is no core agent here to resolve one.
	if *hostname == "" {
		fmt.Fprintln(os.Stderr, "-hostname is required: without it the intake cannot attribute events")
		os.Exit(2)
	}

	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "DD_API_KEY is required; run under: dd-auth --domain dd.datad0g.com -- ...")
		os.Exit(2)
	}

	cfg := pkgconfigsetup.Datadog()
	cfg.Set("api_key", apiKey, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("site", *site, pkgconfigmodel.SourceAgentRuntime)
	pkgconfigsetup.SystemProbe().Set(
		"runtime_security_config.use_secruntime_track", true, pkgconfigmodel.SourceAgentRuntime)

	if err := logsetup.SetupLogger("CWS-MACOS-POC", *logLevel, "", "", false, true, false, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}

	// Staging only. This PoC ships real developer-laptop process activity, so it
	// must never be pointed at a production org.
	if *site != "datad0g.com" {
		log.Warnf("site is %q, not datad0g.com; this PoC is intended for staging only", *site)
	}

	stopper := startstop.NewSerialStopper()
	defer stopper.Stop()

	sender, err := module.NewDirectEventMsgSender(
		stopper,
		logscompression.NewComponent(),
		*hostname,
		// A noop secrets component rather than nil: the reporter dereferences it.
		&secretnooptypes.SecretNoop{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sender: %v\n", err)
		os.Exit(1)
	}

	for _, status := range sender.GetEndpointsStatus() {
		log.Info(status)
	}

	collector, err := darwin.NewCollector(darwin.CollectorConfig{
		Events:      events,
		PoliciesDir: *policiesDir,
		Hostname:    *hostname,
		Sender:      sender,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "collector: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := collector.Run(ctx); err != nil {
		log.Errorf("collector: %v", err)
		os.Exit(1)
	}
}
