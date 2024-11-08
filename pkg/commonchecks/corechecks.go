// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package commonchecks contains shared checks for multiple agent components
package commonchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/network"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/ntp"
	ciscosdwan "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkpath"
	nvidia "github.com/DataDog/datadog-agent/pkg/collector/corechecks/nvidia/jetson"
	oracle "github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/orchestrator/ecs"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/orchestrator/pod"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/sbom"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery"
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
)

// RegisterChecks registers all core checks
func RegisterChecks(store workloadmeta.Component, tagger tagger.Component, cfg config.Component, telemetry telemetry.Component) {
	// Required checks
	corecheckLoader.RegisterCheck(cpu.CheckName, cpu.Factory())
	corecheckLoader.RegisterCheck(memory.CheckName, memory.Factory())
	corecheckLoader.RegisterCheck(uptime.CheckName, uptime.Factory())
	corecheckLoader.RegisterCheck(telemetryCheck.CheckName, telemetryCheck.Factory(telemetry))
	corecheckLoader.RegisterCheck(ntp.CheckName, ntp.Factory())
	corecheckLoader.RegisterCheck(snmp.CheckName, snmp.Factory())
	corecheckLoader.RegisterCheck(networkpath.CheckName, networkpath.Factory(telemetry))
	corecheckLoader.RegisterCheck(io.CheckName, io.Factory())
	corecheckLoader.RegisterCheck(filehandles.CheckName, filehandles.Factory())
	corecheckLoader.RegisterCheck(containerimage.CheckName, containerimage.Factory(store, tagger))
	corecheckLoader.RegisterCheck(containerlifecycle.CheckName, containerlifecycle.Factory(store))
	corecheckLoader.RegisterCheck(generic.CheckName, generic.Factory(store, tagger))

	// Flavor specific checks
	corecheckLoader.RegisterCheck(load.CheckName, load.Factory())
	corecheckLoader.RegisterCheck(kubernetesapiserver.CheckName, kubernetesapiserver.Factory(tagger))
	corecheckLoader.RegisterCheck(ksm.CheckName, ksm.Factory())
	corecheckLoader.RegisterCheck(helm.CheckName, helm.Factory())
	corecheckLoader.RegisterCheck(pod.CheckName, pod.Factory(store, cfg, tagger))
	corecheckLoader.RegisterCheck(ebpf.CheckName, ebpf.Factory())
	corecheckLoader.RegisterCheck(gpu.CheckName, gpu.Factory())
	corecheckLoader.RegisterCheck(ecs.CheckName, ecs.Factory(store, tagger))
	corecheckLoader.RegisterCheck(oomkill.CheckName, oomkill.Factory(tagger))
	corecheckLoader.RegisterCheck(tcpqueuelength.CheckName, tcpqueuelength.Factory(tagger))
	corecheckLoader.RegisterCheck(apm.CheckName, apm.Factory())
	corecheckLoader.RegisterCheck(process.CheckName, process.Factory())
	corecheckLoader.RegisterCheck(network.CheckName, network.Factory())
	corecheckLoader.RegisterCheck(nvidia.CheckName, nvidia.Factory())
	corecheckLoader.RegisterCheck(oracle.CheckName, oracle.Factory())
	corecheckLoader.RegisterCheck(oracle.OracleDbmCheckName, oracle.Factory())
	corecheckLoader.RegisterCheck(disk.CheckName, disk.Factory())
	corecheckLoader.RegisterCheck(wincrashdetect.CheckName, wincrashdetect.Factory())
	corecheckLoader.RegisterCheck(winkmem.CheckName, winkmem.Factory())
	corecheckLoader.RegisterCheck(winproc.CheckName, winproc.Factory())
	corecheckLoader.RegisterCheck(systemd.CheckName, systemd.Factory())
	corecheckLoader.RegisterCheck(orchestrator.CheckName, orchestrator.Factory(store, cfg, tagger))
	corecheckLoader.RegisterCheck(docker.CheckName, docker.Factory(store, tagger))
	corecheckLoader.RegisterCheck(sbom.CheckName, sbom.Factory(store, cfg, tagger))
	corecheckLoader.RegisterCheck(kubelet.CheckName, kubelet.Factory(store, tagger))
	corecheckLoader.RegisterCheck(containerd.CheckName, containerd.Factory(store, tagger))
	corecheckLoader.RegisterCheck(cri.CheckName, cri.Factory(store, tagger))
	corecheckLoader.RegisterCheck(ciscosdwan.CheckName, ciscosdwan.Factory())
	corecheckLoader.RegisterCheck(servicediscovery.CheckName, servicediscovery.Factory())
}
