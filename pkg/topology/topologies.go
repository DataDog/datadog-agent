package topology

// Topology is a batch of instance topology
type Topology struct {
	StartSnapshot bool `json:"start_snapshot"`
	StopSnapshot bool `json:"stop_snapshot"`
	Instance Instance `json:"instance"`
	Components []Component `json:"components"`
	Relations []Relation `json:"relations"`
}
