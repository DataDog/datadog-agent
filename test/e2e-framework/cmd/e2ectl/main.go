// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// e2ectl is the QA-environments CLI: it creates named environments, installs
// agents on them, runs local iteration loops, and inspects the fakeintake.
//
// This binary is Pulumi-free, including agent installation and local kind
// operations. Pulumi-backed infrastructure and fakeintake provisioning run in
// the e2ectl-worker child process. Discovery and config generation are offline.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "environments":
		err = cmdEnvironments(os.Args[2:])
	case "init":
		err = cmdInit(os.Args[2:])
	case "start":
		err = cmdStart(os.Args[2:])
	case "list":
		err = cmdList(os.Args[2:])
	case "install":
		err = cmdInstall(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "fakeintake":
		err = cmdFakeintake(os.Args[2:])
	case "stop":
		err = cmdStop(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("e2ectl dev (schema 1)")
		return
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2ectl: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `e2ectl — QA environments for the Datadog agent

Usage:
  e2ectl environments [--json]                     list available environment types
  e2ectl init --base <type> [--output <file>]       generate a starter config (default: stdout)
  e2ectl start --config <file> --name <name>        create a named environment
  e2ectl list                                      list my created environments
  e2ectl install --env <name> [--config <file>]     install the agent on it
  e2ectl update --env <name> [--skip-build]         rebuild agent code and redeploy (kind)
  e2ectl fakeintake <names|metrics|health> --env <name>
  e2ectl stop --env <name>                          destroy the environment

Get started:
  e2ectl environments
  e2ectl init --base kind --output my-kind.yaml
  # Review/edit my-kind.yaml, then:
  e2ectl start --config my-kind.yaml --name my-kind
  e2ectl install --env my-kind

Discovery and config generation need no credentials and create no infrastructure.
Config generation never overwrites an existing file.
The store lives in $E2ECTL_HOME (default ~/.e2ectl).
`)
}
