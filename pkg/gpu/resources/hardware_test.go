// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// TestNVLinkPeersOnRealHardware dumps what NVML reports on the host, next to the resource
// fields built from it, so they can be compared with `nvidia-smi topo -m`, `topo -p2p` and
// `nvlink -s`. It only runs on a host with NVIDIA GPUs, when GPU_TOPOLOGY_HARDWARE_DUMP is set,
// and prints one JSON document through the test log.
func TestNVLinkPeersOnRealHardware(t *testing.T) {
	if os.Getenv("GPU_TOPOLOGY_HARDWARE_DUMP") == "" {
		t.Skip("set GPU_TOPOLOGY_HARDWARE_DUMP=1 on a GPU host to dump the NVLink and P2P facts")
	}
	cache := safenvml.NewDeviceCache()
	require.NoError(t, cache.Refresh())
	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)

	type rawPort struct {
		Port       int      `json:"port"`
		State      string   `json:"state,omitempty"`
		RemoteType string   `json:"remote_type,omitempty"`
		RemoteBDF  string   `json:"remote_bdf,omitempty"`
		Version    int      `json:"version,omitempty"`
		Errors     []string `json:"errors,omitempty"`
	}
	type rawGPU struct {
		Index int                       `json:"index"`
		UUID  string                    `json:"uuid"`
		BDF   string                    `json:"bdf"`
		Ports []rawPort                 `json:"ports"`
		P2P   map[string]map[string]int `json:"p2p_raw_status"` // peer index -> r/w/a/n/p -> NVML status
	}
	p2pNames := map[nvml.GpuP2PCapsIndex]string{
		nvml.P2P_CAPS_INDEX_READ: "r", nvml.P2P_CAPS_INDEX_WRITE: "w", nvml.P2P_CAPS_INDEX_ATOMICS: "a",
		nvml.P2P_CAPS_INDEX_NVLINK: "n", nvml.P2P_CAPS_INDEX_PCI: "p",
	}

	var raw []rawGPU
	for _, device := range devices {
		physical := device.(*safenvml.PhysicalDevice)
		gpu := rawGPU{Index: physical.Index, UUID: physical.UUID, BDF: physical.PCIBusID, P2P: map[string]map[string]int{}}
		for link := range physical.NVLinkLinkCount {
			port := rawPort{Port: link}
			if state, err := physical.GetNvLinkState(link); err == nil {
				port.State = map[nvml.EnableState]string{nvml.FEATURE_ENABLED: "active", nvml.FEATURE_DISABLED: "inactive"}[state]
			} else {
				port.Errors = append(port.Errors, "state: "+err.Error())
			}
			if deviceType, err := physical.GetNvLinkRemoteDeviceType(link); err == nil {
				port.RemoteType = map[nvml.IntNvLinkDeviceType]string{
					nvml.NVLINK_DEVICE_TYPE_GPU: "gpu", nvml.NVLINK_DEVICE_TYPE_SWITCH: "switch", nvml.NVLINK_DEVICE_TYPE_IBMNPU: "ibmnpu",
				}[deviceType]
			} else {
				port.Errors = append(port.Errors, "remote type: "+err.Error())
			}
			if info, err := physical.GetNvLinkRemotePciInfo(link); err == nil {
				port.RemoteBDF = gpuutil.PCIInfoToBusID(info)
			} else {
				port.Errors = append(port.Errors, "remote pci: "+err.Error())
			}
			if version, err := physical.GetNvLinkVersion(link); err == nil {
				port.Version = version
			}
			gpu.Ports = append(gpu.Ports, port)
		}
		for _, peer := range devices {
			if peer.GetDeviceInfo().UUID == physical.UUID {
				continue
			}
			statuses := map[string]int{}
			for index, name := range p2pNames {
				if status, err := physical.GetP2PStatus(peer, index); err == nil {
					statuses[name] = int(status)
				} else {
					statuses[name] = -1
				}
			}
			gpu.P2P[strconv.Itoa(peer.GetDeviceInfo().Index)] = statuses
		}
		raw = append(raw, gpu)
	}

	peering := NewPeeringScanner(0).Scan(time.Now(), devices)
	complete := map[string]bool{}
	for uuid, p := range peering {
		complete[uuid] = p.Complete
	}
	dump, err := json.MarshalIndent(map[string]any{
		"nvml_raw":         raw,
		"resource_fields":  Snapshot(devices, peering),
		"peering_complete": complete,
	}, "", " ")
	require.NoError(t, err)
	t.Logf("HWDUMP-BEGIN\n%s\nHWDUMP-END", dump)
}
