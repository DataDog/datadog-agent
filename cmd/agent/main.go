// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Main package for the agent binary
package main

import (
	"os"
	"reflect"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	go reflectLoop()
	os.Exit(runcmd.Run(command.MakeCommand(subcommands.AgentSubcommands())))
}

func reflectLoop() {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			for _, name := range reflect.GetAllReflectedMethodNames() {
				if isExported(name) {
					log.Infof("reflect loop: %s", name)
				}
			}
		}
	}
}

func isExported(methodName string) bool {
	for _, r := range methodName {
		return unicode.IsUpper(r)
	}
	return false
}
