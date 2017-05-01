package resources

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Processes interface{}       `json:"processes"`
	Meta      map[string]string `json:"meta"`
}
