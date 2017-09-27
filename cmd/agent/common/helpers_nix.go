// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build freebsd netbsd openbsd solaris dragonfly linux darwin

package common

import (
	"fmt"
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

// SignalList is a slice of the most common unix signals
func SignalList() []os.Signal {
	return []os.Signal{
		syscall.SIGHUP,
		syscall.SIGILL,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGSEGV,
		syscall.SIGKILL,
		syscall.SIGTERM,
		syscall.SIGABRT,
		syscall.SIGPIPE,
		syscall.SIGPROF,
		syscall.SIGTRAP,
		syscall.SIGURG,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
		syscall.SIGALRM,
		syscall.SIGBUS,
		syscall.SIGCHLD,
		syscall.SIGCONT,
		syscall.SIGFPE,
		syscall.SIGIO,
		syscall.SIGIOT,
	}
}

// PanicHandler handles how we log a crash causing panic
// output contains the full output (including stack traces)
func PanicHandler(output string) {
	// output contains the full output (including stack traces)
	err := config.SetupLogger(
		"error", config.Datadog.GetString("log_panic_file"),
		"", false, false, "")

	msg := fmt.Sprintf("Agent panicked (oh no!):\n\n%s\n", output)
	if err == nil {
		log.Error(msg)
		log.Flush()
	} else {
		fmt.Print(msg)
	}

	os.Exit(1)
}
