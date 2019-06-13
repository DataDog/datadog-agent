package topology

// Component is a representation of a topology component
type Component struct {
	ExternalID string                 `json:"externalId"`
	Type       Type                   `json:"type"`
	Data       map[string]interface{} `json:"data"`
}
