// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains shared functionality for multiple subcommands
package common

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/helm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/kubernetesapiserver"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containerimage"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/containerd"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/docker"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/oomkill"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/tcpqueuelength"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/network"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/ntp"
	nvidia "github.com/DataDog/datadog-agent/pkg/collector/corechecks/nvidia/jetson"
	oracle "github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/orchestrator/pod"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/sbom"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/load"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/disk"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/io"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winkmem"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winproc"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/systemd"
	telemetryCheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/telemetry"
	windowsEvent "github.com/DataDog/datadog-agent/pkg/collector/corechecks/windows_event_log"
)

// RegisterChecks registers all core checks
func RegisterChecks(store workloadmeta.Component) {
	// Required checks
	registerCheck(true, cpu.CheckName, cpu.Factory)
	registerCheck(true, memory.CheckName, memory.Factory)
	registerCheck(true, uptime.CheckName, uptime.Factory)
	registerCheck(true, telemetryCheck.CheckName, telemetryCheck.Factory)
	registerCheck(true, ntp.CheckName, ntp.Factory)
	registerCheck(true, snmp.CheckName, snmp.Factory)
	registerCheck(true, io.CheckName, io.Factory)
	registerCheck(true, filehandles.CheckName, filehandles.Factory)
	registerCheck(true, containerimage.CheckName, containerimage.NewFactory(store))
	registerCheck(true, containerlifecycle.CheckName, containerlifecycle.NewFactory(store))
	registerCheck(true, generic.CheckName, generic.NewFactory(store))

	// Flavor specific checks
	registerCheck(load.Enabled, load.CheckName, load.Factory)
	registerCheck(kubernetesapiserver.Enabled, kubernetesapiserver.CheckName, kubernetesapiserver.Factory)
	registerCheck(ksm.Enabled, ksm.CheckName, ksm.Factory)
	registerCheck(helm.Enabled, helm.CheckName, helm.Factory)
	registerCheck(pod.Enabled, pod.CheckName, pod.Factory)
	registerCheck(ebpf.Enabled, ebpf.CheckName, ebpf.Factory)
	registerCheck(oomkill.Enabled, oomkill.CheckName, oomkill.Factory)
	registerCheck(tcpqueuelength.Enabled, tcpqueuelength.CheckName, tcpqueuelength.Factory)
	registerCheck(apm.Enabled, apm.CheckName, apm.Factory)
	registerCheck(process.Enabled, process.CheckName, process.Factory)
	registerCheck(network.Enabled, network.CheckName, network.Factory)
	registerCheck(nvidia.Enabled, nvidia.CheckName, nvidia.Factory)
	registerCheck(oracle.Enabled, oracle.CheckName, oracle.Factory)
	registerCheck(disk.Enabled, disk.CheckName, disk.Factory)
	registerCheck(wincrashdetect.Enabled, wincrashdetect.CheckName, wincrashdetect.Factory)
	registerCheck(winkmem.Enabled, winkmem.CheckName, winkmem.Factory)
	registerCheck(winproc.Enabled, winproc.CheckName, winproc.Factory)
	registerCheck(systemd.Enabled, systemd.CheckName, systemd.Factory)
	registerCheck(windowsEvent.Enabled, windowsEvent.CheckName, windowsEvent.Factory)
	registerCheck(orchestrator.Enabled, orchestrator.CheckName, orchestrator.Factory)
	registerCheck(docker.Enabled, docker.CheckName, docker.NewFactory(store))
	registerCheck(sbom.Enabled, sbom.CheckName, sbom.NewFactory(store))
	registerCheck(kubelet.Enabled, kubelet.CheckName, kubelet.NewFactory(store))
	registerCheck(containerd.Enabled, containerd.CheckName, containerd.NewFactory(store))
	registerCheck(cri.Enabled, cri.CheckName, cri.NewFactory(store))
}

func registerCheck(enabled bool, name string, factory func() check.Check) {
	if enabled {
		corecheckLoader.RegisterCheck(name, factory)
	}
}
