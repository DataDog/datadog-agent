package topology

// Relation is a representation of a topology relation
type Relation struct {
	ExternalID string                 `json:"externalId"`
	SourceID   string                 `json:"sourceId"`
	TargetID   string                 `json:"targetId"`
	Type       Type                   `json:"type"`
	Data       map[string]interface{} `json:"data"`
}
