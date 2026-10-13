// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package run

import "github.com/DataDog/datadog-agent/pkg/config/model"

// macOS: com.datadoghq.procmgr is a system LaunchDaemon. The core Agent runs as _dd-agent and
// cannot modify the system domain (root required). Install/upgrade (installStableJobs) applies
// process_manager.enabled; launchd RunAtLoad keeps the daemon up. No Agent-side launchctl here.

func startDependentServicesAsync(_ model.Reader, _ model.Reader) {}

func stopDependentServices(_ model.Reader, _ model.Reader) {}
