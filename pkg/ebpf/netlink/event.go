package netlink

// IPTranslation can be associated with a connection to show show the connection is NAT'd
type IPTranslation struct {
	ReplSrcIP   string
	ReplDstIP   string
	ReplSrcPort uint16
	ReplDstPort uint16
}
