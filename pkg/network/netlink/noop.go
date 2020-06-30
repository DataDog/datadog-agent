// +build linux
// +build !android

package netlink

import "github.com/DataDog/datadog-agent/pkg/network"

type noOpConntracker struct{}

// NewNoOpConntracker creates a conntracker which always returns empty information
func NewNoOpConntracker() Conntracker {
	return &noOpConntracker{}
}

func (*noOpConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	return nil
}

func (*noOpConntracker) DeleteTranslation(c network.ConnectionStats) {

}

func (*noOpConntracker) Close() {}

func (*noOpConntracker) GetStats() map[string]int64 {
	return map[string]int64{
		"noop_conntracker": 0,
	}
}
