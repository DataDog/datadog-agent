package netpath

type TracerouteHop struct {
	TTL       int     `json:"ttl"`
	IpAddress string  `json:"ip_address"`
	Host      string  `json:"host"`
	Duration  float64 `json:"duration"`
	Success   bool    `json:"success"`
}

type Traceroute struct {
	TracerouteSource string          `json:"traceroute_source"`
	Strategy         string          `json:"strategy"`
	Timestamp        int64           `json:"timestamp"`
	AgentHost        string          `json:"agent_host"`
	DestinationHost  string          `json:"destination_host"`
	Hops             []TracerouteHop `json:"hops"`
	//HopsByIpAddress  map[string]TracerouteHop `json:"hops_by_ip_address"`
}

type TracerouteV2 struct {
	TracerouteSource string `json:"traceroute_source"`
	PathID           string `json:"path_id"`
	Strategy         string `json:"strategy"`
	Timestamp        int64  `json:"timestamp"`
	AgentHost        string `json:"agent_host"`
	DestinationHost  string `json:"destination_host"`

	// HOP
	HopID      string  `json:"hop_id"`
	HopTTL     int     `json:"hop_ttl"`
	HopIp      string  `json:"hop_ip"`
	HopHost    string  `json:"hop_host"`
	HopRtt     float64 `json:"hop_rtt"`
	HopSuccess bool    `json:"hop_success"`

	// Prev HOP
	PrevhopID string `json:"prevhop_id"`

	Message string `json:"message"`
	Team    string `json:"team"`
}

func NewTraceroute() *Traceroute {
	return &Traceroute{
		TracerouteSource: "netpath_integration",
		Strategy:         "path_per_event",
		//HopsByIpAddress:  make(map[string]TracerouteHop),
	}
}
