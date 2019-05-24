// +build linux

package netlink

import "github.com/DataDog/datadog-agent/pkg/process/util"

type noOpConntracker struct{}

// NewNoOpConntracker creates a conntracker which always returns empty information
func NewNoOpConntracker() Conntracker {
	return &noOpConntracker{}
}

func (*noOpConntracker) GetTranslationForConn(ip util.Address, port uint16) *IPTranslation {
	return nil
}

func (*noOpConntracker) ClearShortLived() {}

func (*noOpConntracker) Close() {}

func (*noOpConntracker) GetStats() map[string]int64 {
	return map[string]int64{
		"noop_conntracker": 0,
	}
}
