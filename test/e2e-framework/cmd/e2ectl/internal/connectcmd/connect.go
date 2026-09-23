// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectcmd implements `e2ectl connect <env>`: one-time setup so the
// shell can reach an existing environment — a managed ssh-config entry for
// host bases (`ssh <env>`), a merged kubeconfig context for cluster bases
// (`kubectl --context <env>`), plain docker commands for the local base. A
// --print flag shows the plan without writing anything.
package connectcmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
)

// knownBases is the honest list in unknown-base errors.
const knownBases = "local, kind, ec2-host, docker-host, eks"

// Run implements the connect command: it prints a connection card (fakeintake
// URL, env dir) for every base and configures base-specific access.
func Run(args []string, store *envstore.Store) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	printOnly := fs.Bool("print", false, "print what would be configured, without writing anything")
	flagArgs, names := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(names) != 1 {
		if len(names) > 1 {
			return fmt.Errorf("usage: e2ectl connect <env> [--print] (one environment at a time)")
		}
		entries, err := store.List()
		if err != nil {
			return err
		}
		fmt.Println("available environments:")
		if len(entries) == 0 {
			fmt.Println("  (none — e2ectl start --config <env>.yml --name <name>)")
		}
		for _, e := range entries {
			fmt.Printf("  %s (base %s, %s)\n", e.Name, e.Meta.Base, e.Meta.Status)
		}
		return fmt.Errorf("usage: e2ectl connect <env> [--print]")
	}
	entry, err := store.Get(names[0])
	if err != nil {
		return err
	}
	printCard(entry)
	switch entry.Meta.Base {
	case "ec2-host", "docker-host":
		return connectHost(entry, *printOnly)
	case "kind", "eks":
		return connectCluster(entry, *printOnly)
	case "local":
		return connectLocal(entry)
	default:
		return fmt.Errorf("unknown base %q (known bases: %s)", entry.Meta.Base, knownBases)
	}
}

// splitArgs lets flags appear on either side of the environment name
// (`connect dev --print` behaves like `connect --print dev`); the command has
// a single boolean flag, so a plain prefix split is enough.
func splitArgs(args []string) (flagArgs, positional []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}
	return flagArgs, positional
}

// printCard prints the base-independent connection facts.
func printCard(entry envstore.Entry) {
	if entry.Meta.FakeIntakeURL != "" {
		fmt.Printf("fakeintake: %s\n", entry.Meta.FakeIntakeURL)
	}
	fmt.Printf("env dir:    %s\n", entry.Dir)
}

// connectLocal prints the docker commands for a local environment; no
// configuration is needed — the containers are already on the local docker.
func connectLocal(entry envstore.Entry) error {
	fmt.Printf("connect with:\n  docker exec -it %s bash    (agent container)\n", localinfra.AgentContainer(entry.Name))
	fmt.Printf("fakeintake container: %s (docker logs %s)\n",
		localinfra.FakeintakeContainer(entry.Name), localinfra.FakeintakeContainer(entry.Name))
	return nil
}
