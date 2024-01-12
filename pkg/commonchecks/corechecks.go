// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package commonchecks contains shared checks for multiple agent components
package commonchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
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
	corecheckLoader.RegisterCheck(cpu.CheckName, cpu.New)
	corecheckLoader.RegisterCheck(memory.CheckName, memory.New)
	corecheckLoader.RegisterCheck(uptime.CheckName, uptime.New)
	corecheckLoader.RegisterCheck(telemetryCheck.CheckName, telemetryCheck.New)
	corecheckLoader.RegisterCheck(ntp.CheckName, ntp.New)
	corecheckLoader.RegisterCheck(snmp.CheckName, snmp.New)
	corecheckLoader.RegisterCheck(io.CheckName, io.New)
	corecheckLoader.RegisterCheck(filehandles.CheckName, filehandles.New)
	corecheckLoader.RegisterCheck(containerimage.CheckName, containerimage.Factory(store))
	corecheckLoader.RegisterCheck(containerlifecycle.CheckName, containerlifecycle.Factory(store))
	corecheckLoader.RegisterCheck(generic.CheckName, generic.Factory(store))

	// Flavor specific checks
	corecheckLoader.RegisterCheckIfEnabled(load.Enabled, load.CheckName, load.New)
	corecheckLoader.RegisterCheckIfEnabled(kubernetesapiserver.Enabled, kubernetesapiserver.CheckName, kubernetesapiserver.New)
	corecheckLoader.RegisterCheckIfEnabled(ksm.Enabled, ksm.CheckName, ksm.New)
	corecheckLoader.RegisterCheckIfEnabled(helm.Enabled, helm.CheckName, helm.New)
	corecheckLoader.RegisterCheckIfEnabled(pod.Enabled, pod.CheckName, pod.New)
	corecheckLoader.RegisterCheckIfEnabled(ebpf.Enabled, ebpf.CheckName, ebpf.New)
	corecheckLoader.RegisterCheckIfEnabled(oomkill.Enabled, oomkill.CheckName, oomkill.New)
	corecheckLoader.RegisterCheckIfEnabled(tcpqueuelength.Enabled, tcpqueuelength.CheckName, tcpqueuelength.New)
	corecheckLoader.RegisterCheckIfEnabled(apm.Enabled, apm.CheckName, apm.New)
	corecheckLoader.RegisterCheckIfEnabled(process.Enabled, process.CheckName, process.New)
	corecheckLoader.RegisterCheckIfEnabled(network.Enabled, network.CheckName, network.New)
	corecheckLoader.RegisterCheckIfEnabled(nvidia.Enabled, nvidia.CheckName, nvidia.New)
	corecheckLoader.RegisterCheckIfEnabled(oracle.Enabled, oracle.CheckName, oracle.New)
	corecheckLoader.RegisterCheckIfEnabled(disk.Enabled, disk.CheckName, disk.New)
	corecheckLoader.RegisterCheckIfEnabled(wincrashdetect.Enabled, wincrashdetect.CheckName, wincrashdetect.New)
	corecheckLoader.RegisterCheckIfEnabled(winkmem.Enabled, winkmem.CheckName, winkmem.New)
	corecheckLoader.RegisterCheckIfEnabled(winproc.Enabled, winproc.CheckName, winproc.New)
	corecheckLoader.RegisterCheckIfEnabled(systemd.Enabled, systemd.CheckName, systemd.New)
	corecheckLoader.RegisterCheckIfEnabled(windowsEvent.Enabled, windowsEvent.CheckName, windowsEvent.New)
	corecheckLoader.RegisterCheckIfEnabled(orchestrator.Enabled, orchestrator.CheckName, orchestrator.New)
	corecheckLoader.RegisterCheckIfEnabled(docker.Enabled, docker.CheckName, docker.Factory(store))
	corecheckLoader.RegisterCheckIfEnabled(sbom.Enabled, sbom.CheckName, sbom.Factory(store))
	corecheckLoader.RegisterCheckIfEnabled(kubelet.Enabled, kubelet.CheckName, kubelet.Factory(store))
	corecheckLoader.RegisterCheckIfEnabled(containerd.Enabled, containerd.CheckName, containerd.Factory(store))
	corecheckLoader.RegisterCheckIfEnabled(cri.Enabled, cri.CheckName, cri.Factory(store))
}
