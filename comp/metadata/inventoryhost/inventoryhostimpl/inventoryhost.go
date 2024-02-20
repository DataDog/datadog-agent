// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryhostimpl implements a component to generate the 'host_metadata' metadata payload for inventory.
package inventoryhostimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/memory"
	"github.com/DataDog/datadog-agent/pkg/gohai/network"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newInventoryHostProvider))
}

// for testing purpose
var (
	cpuGet        = cpu.CollectInfo
	memoryGet     = memory.CollectInfo
	networkGet    = network.CollectInfo
	platformGet   = platform.CollectInfo
	osVersionGet  = utils.GetOSVersion
	pkgSigningGet = pkgUtils.GetLinuxGlobalSigningPolicies
)

// hostMetadata contains metadata about the host
type hostMetadata struct {
	// from gohai/cpu
	CPUCores             uint64  `json:"cpu_cores"`
	CPULogicalProcessors uint64  `json:"cpu_logical_processors"`
	CPUVendor            string  `json:"cpu_vendor"`
	CPUModel             string  `json:"cpu_model"`
	CPUModelID           string  `json:"cpu_model_id"`
	CPUFamily            string  `json:"cpu_family"`
	CPUStepping          string  `json:"cpu_stepping"`
	CPUFrequency         float64 `json:"cpu_frequency"`
	CPUCacheSize         uint64  `json:"cpu_cache_size"`

	// from gohai/platform
	KernelName      string `json:"kernel_name"`
	KernelRelease   string `json:"kernel_release"`
	KernelVersion   string `json:"kernel_version"`
	OS              string `json:"os"`
	CPUArchitecture string `json:"cpu_architecture"`

	// from gohai/memory
	MemoryTotalKb     uint64 `json:"memory_total_kb"`
	MemorySwapTotalKb uint64 `json:"memory_swap_total_kb"`

	// from gohai/network
	IPAddress   string `json:"ip_address"`
	IPv6Address string `json:"ipv6_address"`
	MacAddress  string `json:"mac_address"`

	// from the agent itself
	AgentVersion           string `json:"agent_version"`
	CloudProvider          string `json:"cloud_provider"`
	CloudProviderSource    string `json:"cloud_provider_source"`
	CloudProviderAccountID string `json:"cloud_provider_account_id"`
	CloudProviderHostID    string `json:"cloud_provider_host_id"`
	OsVersion              string `json:"os_version"`

	// from file system
	HypervisorGuestUUID string `json:"hypervisor_guest_uuid"`
	DmiProductUUID      string `json:"dmi_product_uuid"`
	DmiBoardAssetTag    string `json:"dmi_board_asset_tag"`
	DmiBoardVendor      string `json:"dmi_board_vendor"`

	// from package repositories
	LinuxPackageSigningEnabled   bool `json:"linux_package_signing_enabled"`
	RPMGlobalRepoGPGCheckEnabled bool `json:"rpm_global_repo_gpg_check_enabled"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string        `json:"hostname"`
	Timestamp int64         `json:"timestamp"`
	Metadata  *hostMetadata `json:"host_metadata"`
	UUID      string        `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories host payload any more, payload is too big for intake")
}

type invHost struct {
	util.InventoryPayload

	log      log.Component
	conf     config.Component
	data     *hostMetadata
	hostname string
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
}

type provides struct {
	fx.Out

	Comp          inventoryhost.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
}

func newInventoryHostProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	ih := &invHost{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: hname,
		data:     &hostMetadata{},
	}
	ih.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, ih.getPayload, "host.json")

	return provides{
		Comp:          ih,
		Provider:      ih.MetadataProvider(),
		FlareProvider: ih.FlareProvider(),
	}
}

func (ih *invHost) fillData() {
	logWarnings := func(warnings []string) {
		for _, w := range warnings {
			ih.log.Infof("gohai: %s", w)
		}
	}

	cpuInfo := cpuGet()
	_, warnings, err := cpuInfo.AsJSON()
	if err != nil {
		ih.log.Errorf("Failed to retrieve cpu metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		ih.data.CPUCores = cpuInfo.CPUCores.ValueOrDefault()
		ih.data.CPULogicalProcessors = cpuInfo.CPULogicalProcessors.ValueOrDefault()
		ih.data.CPUVendor = cpuInfo.VendorID.ValueOrDefault()
		ih.data.CPUModel = cpuInfo.ModelName.ValueOrDefault()
		ih.data.CPUModelID = cpuInfo.Model.ValueOrDefault()
		ih.data.CPUFamily = cpuInfo.Family.ValueOrDefault()
		ih.data.CPUStepping = cpuInfo.Stepping.ValueOrDefault()
		ih.data.CPUFrequency = cpuInfo.Mhz.ValueOrDefault()
		ih.data.CPUCacheSize = cpuInfo.CacheSizeKB.ValueOrDefault() * 1024
	}

	platformInfo := platformGet()
	_, warnings, err = platformInfo.AsJSON()
	if err != nil {
		ih.log.Errorf("failed to retrieve host platform metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		ih.data.KernelName = platformInfo.KernelName.ValueOrDefault()
		ih.data.KernelRelease = platformInfo.KernelRelease.ValueOrDefault()
		ih.data.KernelVersion = platformInfo.KernelVersion.ValueOrDefault()
		ih.data.OS = platformInfo.OS.ValueOrDefault()
		ih.data.CPUArchitecture = platformInfo.HardwarePlatform.ValueOrDefault()
	}

	memoryInfo := memoryGet()
	_, warnings, err = memoryInfo.AsJSON()
	if err != nil {
		ih.log.Errorf("failed to retrieve host memory metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		ih.data.MemoryTotalKb = memoryInfo.TotalBytes.ValueOrDefault() / 1024
		ih.data.MemorySwapTotalKb = memoryInfo.SwapTotalKb.ValueOrDefault()
	}

	networkInfo, err := networkGet()
	if err == nil {
		_, warnings, err = networkInfo.AsJSON()
	}
	if err != nil {
		ih.log.Errorf("failed to retrieve host network metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		ih.data.IPAddress = networkInfo.IPAddress
		ih.data.IPv6Address = networkInfo.IPAddressV6.ValueOrDefault()
		ih.data.MacAddress = networkInfo.MacAddress
	}

	ih.data.AgentVersion = version.AgentVersion
	ih.data.HypervisorGuestUUID = dmi.GetHypervisorUUID()
	ih.data.DmiProductUUID = dmi.GetProductUUID()
	ih.data.DmiBoardAssetTag = dmi.GetBoardAssetTag()
	ih.data.DmiBoardVendor = dmi.GetBoardVendor()

	cloudProvider, cloudAccountID := cloudproviders.DetectCloudProvider(context.Background(), ih.conf.GetBool("inventories_collect_cloud_provider_account_id"), ih.log)
	ih.data.CloudProvider = cloudProvider
	ih.data.CloudProviderAccountID = cloudAccountID

	ih.data.CloudProviderSource = cloudproviders.GetSource(cloudProvider)
	ih.data.CloudProviderHostID = cloudproviders.GetHostID(context.Background(), cloudProvider)
	ih.data.OsVersion = osVersionGet()

	gpgcheck, repoGPGCheck := pkgSigningGet(ih.log)
	ih.data.LinuxPackageSigningEnabled = gpgcheck
	ih.data.RPMGlobalRepoGPGCheckEnabled = repoGPGCheck
}

func (ih *invHost) getPayload() marshaler.JSONMarshaler {
	ih.fillData()

	return &Payload{
		Hostname:  ih.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  ih.data,
		UUID:      uuid.GetUUID(),
	}
}
