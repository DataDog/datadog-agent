package enrichment

var tcpFlagsMapping = map[uint32]string{
	1:  "FIN",
	2:  "SYN",
	4:  "RST",
	8:  "PSH",
	16: "ACK",
	32: "URG",
}

// FormatFCPFlags format TCP Flags from bitmask to strings
func FormatFCPFlags(flags uint32) []string {
	var strFlags []string
	flag := uint32(1)
	for {
		if flag > 32 {
			break
		}
		strFlag, ok := tcpFlagsMapping[flag]
		if !ok {
			continue
		}
		if flag&flags != 0 {
			strFlags = append(strFlags, strFlag)
		}
		flag <<= 1
	}
	return strFlags
}
