// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package configsender ships discovered config-file contents to the EvP
// staging intake (demoalpha track) as part of the DSCVR-438 PoC.
//
// The component polls the system-probe discovery module's /services endpoint
// and, for each Service.ConfigFiles entry it can recognise, reads the file
// and POSTs the envelope expected by the demoalpha-worker.
//
// Opt-in via discovery.config_files_sender.enabled. Linux only.
package configsender

// team: agent-discovery

// Component is the marker interface for the configsender component. The
// component exposes no public methods — instantiation through fx starts the
// background goroutine via a lifecycle hook, and consumption is implicit.
type Component interface{}
