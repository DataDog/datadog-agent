package topology

type Component struct {
	ExternalId string `json:"externalId"`
	Type Type `json:"type"`
	Data map[string]interface{} `json:"data"`
}
