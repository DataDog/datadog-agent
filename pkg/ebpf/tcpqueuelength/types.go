package tcpqueuelength

// StatsKey is the type of the `Stats` map key: the container ID
type StatsKey struct {
	ContainerID string `json:"containerid"`
}

// StatsValue is the type of the `Stats` map value: the maximum fill rate of busiest read and write buffers
type StatsValue struct {
	ReadBufferMaxFillRate  uint32 `json:"read_buffer_max_fill_rate"`
	WriteBufferMaxFillRate uint32 `json:"write_buffer_max_fill_rate"`
}

// Stats is the map of the maximum fill rate of the read and write buffers per container
type Stats map[string]StatsValue
