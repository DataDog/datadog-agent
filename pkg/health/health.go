package health

// Health is a batch of health synchronization data
type Health struct {
	StartSnapshot *StartSnapshotMetadata `json:"start_snapshot,omitempty"`
	StopSnapshot  *StopSnapshotMetadata  `json:"stop_snapshot,omitempty"`
	Stream        Stream                 `json:"stream"`
	CheckStates   []CheckData            `json:"check_states"`
}
