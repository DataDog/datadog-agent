// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// e2ectl is the QA-environments CLI: it creates named environments, installs
// agents on them, runs local iteration loops, and inspects the fakeintake.
//
// The fast, local commands live in this binary (Pulumi-free by design); the
// Pulumi-linked or heavyweight jobs (cloud provisioning, chart installs) run
// in the e2ectl-worker child process.
package main

import (
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2ectl: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `e2ectl — QA environments for the Datadog agent

Usage:
  e2ectl start   --config <env>.yml --name <name>   create a named environment
  e2ectl list                                       list my environments
  e2ectl install --config <env>.yml --env <name>    install the agent on it
  e2ectl update --env <name> [--skip-build]         rebuild agent code and redeploy (kind)
  e2ectl fakeintake <names|metrics|health> --env <name>
  e2ectl stop    --env <name>                        destroy the environment

Environment config files use schema 1, e.g.:

  schema: 1
  environment:
    base: kind          # or ec2-host
    fakeintake: true
  agent:
    install: helm       # helm (kind) or script (ec2-host)
    image: gcr.io/datadoghq/agent:my-dev

The store lives in $E2ECTL_HOME (default ~/.e2ectl).
`)
}
