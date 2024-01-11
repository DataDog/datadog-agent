// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package common contains shared functionality for multiple subcommands
package common

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/helm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/kubernetesapiserver"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/disk"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/io"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winproc"
)

// RegisterChecks registers all core checks
func RegisterChecks() {
	// Required checks
	registerCheck(true, cpu.CheckName, cpu.Factory)
	registerCheck(true, memory.CheckName, memory.Factory)
	registerCheck(true, uptime.CheckName, uptime.Factory)
	registerCheck(true, io.CheckName, io.Factory)
	registerCheck(true, filehandles.CheckName, filehandles.Factory)

	// Flavor specific checks
	registerCheck(kubernetesapiserver.Enabled, kubernetesapiserver.CheckName, kubernetesapiserver.Factory)
	registerCheck(ksm.Enabled, ksm.CheckName, ksm.Factory)
	registerCheck(helm.Enabled, helm.CheckName, helm.Factory)
	registerCheck(disk.Enabled, disk.CheckName, disk.Factory)
	registerCheck(orchestrator.Enabled, orchestrator.CheckName, orchestrator.Factory)
	registerCheck(winproc.Enabled, winproc.CheckName, winproc.Factory)
}

func registerCheck(enabled bool, name string, factory func() check.Check) {
	if enabled {
		corecheckLoader.RegisterCheck(name, factory)
	}
}
