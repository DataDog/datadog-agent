// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ErrNotEnabled represents the case in which datadog-installer is not enabled
var ErrNotEnabled = errors.New("datadog-installer not enabled")

func runFxWrapper(global *command.GlobalParams) error {
	return fxutil.OneShot(
		run,
		getCommonFxOption(global),
	)
}

func run(shutdowner fx.Shutdowner, cfg config.Component, _ pid.Component, _ localapi.Component, _ telemetry.Component) error {
	if err := gracefullyExitIfDisabled(cfg, shutdowner); err != nil {
		log.Infof("Datadog installer is not enabled, exiting")
		return nil
	}
	log.Infof("Running version with debug goroutine stacks enabled")
	releaseMemory()
	handleSignals(shutdowner)
	return nil
}

func gracefullyExitIfDisabled(cfg config.Component, shutdowner fx.Shutdowner) error {
	if !cfg.GetBool("remote_updates") {
		// Note: when not using systemd we may run into an issue where we need to
		// sleep for a while here, like the system probe does
		// See https://github.com/DataDog/datadog-agent/blob/b5c6a93dff27a8fdae37fc9bf23b3604a9f87591/cmd/system-probe/subcommands/run/command.go#L128
		_ = shutdowner.Shutdown()
		return ErrNotEnabled
	}
	return nil
}

func handleSignals(shutdowner fx.Shutdowner) {
	// SIGUSR1: dump goroutine stacks to stderr (for debugging deadlocks without shutting down)
	sigusr1Ch := make(chan os.Signal, 1)
	signal.Notify(sigusr1Ch, syscall.SIGUSR1)
	go func() {
		for range sigusr1Ch {
			DumpGoroutineStacks()
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("Received signal %d (%v), dumping goroutine stacks and shutting down...", signo, signo)
			DumpGoroutineStacks()
			_ = shutdowner.Shutdown()
			return
		}
	}
}

// DumpGoroutineStacks writes all goroutine stacks to /tmp and to stderr. Used
// when handling SIGTERM/SIGINT (before shutdown) and SIGUSR1 (on-demand) to
// debug deadlocks. Syncs to disk so the dump is visible even if the process
// is killed immediately after.
func DumpGoroutineStacks() {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 2); err != nil {
		_, _ = os.Stderr.WriteString("failed to write goroutine profile: " + err.Error() + "\n")
		return
	}
	dump := fmt.Sprintf("\n===== goroutine stack dump %s =====\n%s===== end goroutine stack dump =====\n\n",
		time.Now().Format(time.RFC3339), buf.String())

	name := fmt.Sprintf("datadog-installer-daemon-goroutine-stacks-%s.txt", time.Now().Format("20060102-150405"))
	path := filepath.Join("/tmp", name)
	f, err := os.Create(path)
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to write goroutine dump to " + path + ": " + err.Error() + "\n")
		_, _ = os.Stderr.WriteString(dump)
		return
	}
	_, _ = f.WriteString(dump)
	_ = f.Sync()
	_ = f.Close()
	_, _ = os.Stderr.WriteString("goroutine stack dump written to " + path + "\n")
	_, _ = os.Stderr.WriteString(dump)
}
