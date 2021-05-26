package nat

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// GetLocalAddress returns the translated (local ip, local port) pair
func GetLocalAddress(c network.ConnectionStats) (util.Address, uint16) {
	localIP := c.Source
	localPort := c.SPort

	if c.IPTranslation != nil && c.IPTranslation.ReplDstIP != nil {
		// Fields are flipped
		localIP = c.IPTranslation.ReplDstIP
		localPort = c.IPTranslation.ReplDstPort
	}
	return localIP, localPort
}

// GetRemoteAddress returns the translated (remote ip, remote port) pair
func GetRemoteAddress(c network.ConnectionStats) (util.Address, uint16) {
	remoteIP := c.Dest
	remotePort := c.DPort

	if c.IPTranslation != nil && c.IPTranslation.ReplSrcIP != nil {
		// Fields are flipped
		remoteIP = c.IPTranslation.ReplSrcIP
		remotePort = c.IPTranslation.ReplSrcPort
	}
	return remoteIP, remotePort
}
