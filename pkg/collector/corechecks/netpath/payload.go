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

type NetworkPathHop struct {
	TTL       int     `json:"ttl"`
	IpAddress string  `json:"ip_address"`
	Hostname  string  `json:"hostname"`
	RTT       float64 `json:"rtt"`
	Success   bool    `json:"success"`
}

type NetworkPathSource struct {
	Hostname string `json:"hostname"`
}

type NetworkPathDestination struct {
	Hostname  string `json:"hostname"`
	IpAddress string `json:"ip_address"`
}

type NetworkPath struct {
	Timestamp   int64                  `json:"timestamp"`
	PathId      string                 `json:"path_id"`
	Source      NetworkPathSource      `json:"source"`
	Destination NetworkPathDestination `json:"destination"`
	Hops        []NetworkPathHop       `json:"hops"`
}
