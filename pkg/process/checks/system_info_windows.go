package checks

import (
	"fmt"
	"strconv"
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/platform"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// CollectSystemInfo collects a set of system-level information that will not
// change until a restart. This bit of information should be passed along with
// the process messages.
func CollectSystemInfo(cfg *config.AgentConfig) (*model.SystemInfo, error) {
	hi, err := platform.GetArchInfo()
	if err != nil {
		return nil, err
	}
	cpuInfo, err := cpu.GetCpuInfo()
	if err != nil {
		return nil, err
	}
	fmt.Printf("cpu info %v\n", cpuInfo)
	mi, err := winutil.VirtualMemory()
	if err != nil {
		return nil, err
	}
	phys_count, err := strconv.ParseInt(cpuInfo["cpu_pkgs"], 10, 64)
	logical_count, err := strconv.ParseInt(cpuInfo["cpu_logical_processors"], 10, 64)
	clock_speed, err := strconv.ParseInt(cpuInfo["mhz"], 10, 64)
	l2_cache, err := strconv.ParseInt(cpuInfo["cache_size_l2"], 10, 64)
	cpus := make([]*model.CPUInfo, 0)
	for i := int64(0) ; i < phys_count ; i++ {
		cpus = append(cpus, &model.CPUInfo{
			Number:     int32(i),
			Vendor:     cpuInfo["vendor_id"],
			Family:     cpuInfo["family"],
			Model:      cpuInfo["model"],
			PhysicalId: "",
			CoreId:     "",
			Cores:      int32(logical_count),
			Mhz:        int64(clock_speed),
			CacheSize:  int32(l2_cache),
		})
	}

	m := &model.SystemInfo{
		Uuid: "",
		Os: &model.OSInfo{
			Name:          hi["kernel_name"].(string),
			Platform:      hi["os"].(string),
			Family:        hi["family"].(string),
			Version:       hi["kernel_release"].(string),
			KernelVersion: "",
		},
		Cpus:        cpus,
		TotalMemory: int64(mi.Total),
	}
	return m, nil
}
