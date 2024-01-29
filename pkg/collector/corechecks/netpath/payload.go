package netpath

type TracerouteV2 struct {
	PathId           string  `json:"path_id"`
	TracerouteSource string  `json:"traceroute_source"`
	Timestamp        int64   `json:"timestamp"`
	AgentHost        string  `json:"agent_host"`
	DestinationHost  string  `json:"destination_host"`
	TTL              int     `json:"ttl"`
	IpAddress        string  `json:"ip_address"`
	Host             string  `json:"host"`
	Duration         float64 `json:"duration"`
	Success          bool    `json:"success"`
}
