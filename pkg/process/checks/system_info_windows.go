package checks

import (
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/platform"
	"strconv"

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
	mi, err := winutil.VirtualMemory()
	if err != nil {
		return nil, err
	}
	physCount, _ := strconv.ParseInt(cpuInfo["cpu_pkgs"], 10, 64)
	logicalCount, _ := strconv.ParseInt(cpuInfo["cpu_logical_processors"], 10, 64)
	clockSpeed, _ := strconv.ParseInt(cpuInfo["mhz"], 10, 64)
	l2Cache, _ := strconv.ParseInt(cpuInfo["cache_size_l2"], 10, 64)
	cpus := make([]*model.CPUInfo, 0)
	for i := int64(0); i < physCount; i++ {
		cpus = append(cpus, &model.CPUInfo{
			Number:     int32(i),
			Vendor:     cpuInfo["vendor_id"],
			Family:     cpuInfo["family"],
			Model:      cpuInfo["model"],
			PhysicalId: "",
			CoreId:     "",
			Cores:      int32(logicalCount),
			Mhz:        int64(clockSpeed),
			CacheSize:  int32(l2Cache),
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
