// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/receiver"
)

func cmdReceiver(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("receiver requires plan, apply, status or serve")
	}
	if args[0] == "serve" {
		return serveReceiver(args[1:])
	}
	sub := args[0]
	if sub != "plan" && sub != "apply" && sub != "status" {
		return fmt.Errorf("unknown receiver operation")
	}
	fs := flag.NewFlagSet("receiver "+sub, flag.ContinueOnError)
	name := fs.String("env", "", "environment name")
	path := fs.String("config", "", "candidate config (defaults to stored config)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *name == "" || fs.NArg() != 0 {
		return fmt.Errorf("--env is required; positional arguments are not supported")
	}
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Get(*name)
	if err != nil {
		return err
	}
	if sub == "status" {
		state, err := installer.RoutingStatus(entry)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(state)
	}
	if sub == "apply" {
		unlock, err := entry.LockInstallation()
		if err != nil {
			return err
		}
		defer unlock()
	}
	cfg, err := loadOrStoredConfig(*path, entry)
	if err != nil {
		return err
	}
	d, err := driver.Get(cfg.Environment.Base)
	if err != nil {
		return err
	}
	inst, err := driver.InstallerFor(d, cfg.Agent.Install)
	if err != nil {
		return err
	}
	if err := installer.ValidateReceiver(inst, cfg); err != nil {
		return err
	}
	if errs := inst.Validate(cfg); len(errs) > 0 {
		return config.NewErrors(errs)
	}
	planner, ok := inst.(installer.RoutingConsumer)
	if !ok {
		return fmt.Errorf("installer does not consume receivers")
	}
	// Environment-provided sinks are reconciled before resolution: switching to
	// a managed blackhole starts the sink, switching away removes it. Planning
	// stays read-only and reports a missing managed sink instead of creating one.
	if sub == "apply" {
		if err := installer.SyncManagedSink(d.SinkSyncer(), cfg, entry); err != nil {
			return err
		}
	}
	plan, err := planner.PrepareRouting(cfg, entry)
	if err != nil {
		return err
	}
	if sub == "plan" {
		return json.NewEncoder(os.Stdout).Encode(plan)
	}
	applier, ok := inst.(installer.RoutingApplier)
	if !ok {
		return fmt.Errorf("installer %s does not support no-build receiver apply; no install fallback is performed", inst.ID())
	}
	if err := installer.WithRoutingState(inst, cfg, entry, func() error { return applier.ApplyRouting(cfg, entry) }); err != nil {
		entry.Meta.AgentInstalled = false
		_ = store.UpdateMeta(entry)
		return err
	}
	if err := saveAppliedConfig(cfg, entry); err != nil {
		return err
	}
	entry.Meta.AgentInstalled = true
	return store.UpdateMeta(entry)
}

// Serving is a registry capability, not a receiver-type switch. The process is
// operator-owned and must remain running/reachable for as long as Agents use it.
func serveReceiver(args []string) error {
	fs := flag.NewFlagSet("receiver serve", flag.ContinueOnError)
	typ := fs.String("type", "", "receiver type with a serve capability")
	listen := fs.String("listen", "127.0.0.1:8080", "HTTP listen address (choose a producer-reachable interface)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("serve takes no positional arguments")
	}
	d, err := receiver.Get(*typ)
	if err != nil {
		return err
	}
	if d.Serve == nil {
		return fmt.Errorf("receiver %s has no serve capability", d.ID)
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: d.Serve(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-done:
		}
	}()
	fmt.Fprintf(os.Stderr, "Serving %s on %s; operator-owned lifetime, no payload retention/logging. RC unsupported.\n", d.ID, listener.Addr())
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
