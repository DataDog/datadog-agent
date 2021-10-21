package network

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
)

var (
	ephemeralLow  = uint16(0)
	ephemeralHigh = uint16(0)

	initEphemeralIntPair sync.Once
	ephemeralIntPair     *sysctl.IntPair
)

// IsPortInEphemeralRange returns whether the port is ephemeral based on the OS-specific configuration.
func IsPortInEphemeralRange(p uint16) EphemeralPortType {
	initEphemeralIntPair.Do(func() {
		procfsPath := "/proc"
		if config.Datadog.IsSet("procfs_path") {
			procfsPath = config.Datadog.GetString("procfs_path")
		}
		ephemeralIntPair = sysctl.NewIntPair(procfsPath, "net/ipv4/ip_local_port_range", time.Hour)
	})

	low, hi, err := ephemeralIntPair.Get()
	if err == nil {
		ephemeralLow = uint16(low)
		ephemeralHigh = uint16(hi)
	}
	if err != nil || ephemeralLow == 0 || ephemeralHigh == 0 {
		return EphemeralUnknown
	}
	if p >= ephemeralLow && p <= ephemeralHigh {
		return EphemeralTrue
	}
	return EphemeralFalse
}
