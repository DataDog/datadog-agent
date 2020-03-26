// +build linux

package netlink

import (
	"github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type noOpConntracker struct{}

// NewNoOpConntracker creates a conntracker which always returns empty information
func NewNoOpConntracker() Conntracker {
	return &noOpConntracker{}
}

func (*noOpConntracker) GetTranslationForConn(
	srcIP util.Address,
	srcPort uint16,
	dstIP util.Address,
	dstPort uint16,
	transport process.ConnectionType,
) *IPTranslation {
	return nil
}

func (*noOpConntracker) ClearShortLived() {}

func (*noOpConntracker) Close() {}

func (*noOpConntracker) GetStats() map[string]int64 {
	return map[string]int64{
		"noop_conntracker": 0,
	}
}
