package health

// StartSnapshotMetadata is a representation of 'start' for health synchronization
type StartSnapshotMetadata struct {
	RepeatIntervalS int `json:"repeat_interval_s"`
	ExpiryIntervalS int `json:"expiry_interval_s,omitempty"`
}
