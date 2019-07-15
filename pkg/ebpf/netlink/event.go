package netlink

import "github.com/DataDog/datadog-agent/pkg/process/util"

// IPTranslation can be associated with a connection to show show the connection is NAT'd
type IPTranslation struct {
	ReplSrcIP   util.Address
	ReplDstIP   util.Address
	ReplSrcPort uint16
	ReplDstPort uint16
}
