package topology

type Relation struct {
	ExternalId string `json:"externalId"`
	SourceId string `json:"sourceId"`
	TargetId string `json:"targetId"`
	Type Type `json:"type"`
	Data map[string]interface{} `json:"data"`
}
