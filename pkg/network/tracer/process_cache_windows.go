// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

<<<<<<<< HEAD:cmd/agent/subcommands/run/command_snmptraps_iot.go
//go:build jetson

package run

import "go.uber.org/fx"

func getSnmptrapsOptions() fx.Option {
	return fx.Options()
========
//go:build windows && npm

package tracer

import "github.com/DataDog/datadog-agent/pkg/network/events"

func rescueEventWithProcfs(_ *events.Process) *events.Process {
	return nil
>>>>>>>> main:pkg/network/tracer/process_cache_windows.go
}
