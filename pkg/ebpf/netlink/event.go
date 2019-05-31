package netlink

// IPTranslation can be associated with a connection to show show the connection is NAT'd
//easyjson:json
type IPTranslation struct {
	ReplSrcIP   string `json:"r_src"`
	ReplDstIP   string `json:"r_dst"`
	ReplSrcPort uint16 `json:"r_sport"`
	ReplDstPort uint16 `json:"r_dport"`
}
