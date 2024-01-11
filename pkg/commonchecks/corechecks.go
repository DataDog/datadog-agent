// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package commonchecks contains shared checks for multiple agent components
package commonchecks

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
	registerCheck(true, cpu.CheckName, cpu.New)
	registerCheck(true, memory.CheckName, memory.New)
	registerCheck(true, uptime.CheckName, uptime.New)
	registerCheck(true, telemetryCheck.CheckName, telemetryCheck.New)
	registerCheck(true, ntp.CheckName, ntp.New)
	registerCheck(true, snmp.CheckName, snmp.New)
	registerCheck(true, io.CheckName, io.New)
	registerCheck(true, filehandles.CheckName, filehandles.New)
	registerCheck(true, containerimage.CheckName, containerimage.Factory(store))
	registerCheck(true, containerlifecycle.CheckName, containerlifecycle.Factory(store))
	registerCheck(true, generic.CheckName, generic.Factory(store))

	// Flavor specific checks
	registerCheck(load.Enabled, load.CheckName, load.New)
	registerCheck(kubernetesapiserver.Enabled, kubernetesapiserver.CheckName, kubernetesapiserver.New)
	registerCheck(ksm.Enabled, ksm.CheckName, ksm.New)
	registerCheck(helm.Enabled, helm.CheckName, helm.New)
	registerCheck(pod.Enabled, pod.CheckName, pod.New)
	registerCheck(ebpf.Enabled, ebpf.CheckName, ebpf.New)
	registerCheck(oomkill.Enabled, oomkill.CheckName, oomkill.New)
	registerCheck(tcpqueuelength.Enabled, tcpqueuelength.CheckName, tcpqueuelength.New)
	registerCheck(apm.Enabled, apm.CheckName, apm.New)
	registerCheck(process.Enabled, process.CheckName, process.New)
	registerCheck(network.Enabled, network.CheckName, network.New)
	registerCheck(nvidia.Enabled, nvidia.CheckName, nvidia.New)
	registerCheck(oracle.Enabled, oracle.CheckName, oracle.New)
	registerCheck(disk.Enabled, disk.CheckName, disk.New)
	registerCheck(wincrashdetect.Enabled, wincrashdetect.CheckName, wincrashdetect.New)
	registerCheck(winkmem.Enabled, winkmem.CheckName, winkmem.New)
	registerCheck(winproc.Enabled, winproc.CheckName, winproc.New)
	registerCheck(systemd.Enabled, systemd.CheckName, systemd.New)
	registerCheck(windowsEvent.Enabled, windowsEvent.CheckName, windowsEvent.New)
	registerCheck(orchestrator.Enabled, orchestrator.CheckName, orchestrator.New)
	registerCheck(docker.Enabled, docker.CheckName, docker.Factory(store))
	registerCheck(sbom.Enabled, sbom.CheckName, sbom.Factory(store))
	registerCheck(kubelet.Enabled, kubelet.CheckName, kubelet.Factory(store))
	registerCheck(containerd.Enabled, containerd.CheckName, containerd.Factory(store))
	registerCheck(cri.Enabled, cri.CheckName, cri.Factory(store))
}

func registerCheck(enabled bool, name string, factory func() check.Check) {
	if enabled {
		corecheckLoader.RegisterCheck(name, factory)
	}
}
